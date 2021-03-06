package publish

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"github.com/lbryio/lbrytv/app/auth"
	"github.com/lbryio/lbrytv/app/proxy"
	"github.com/lbryio/lbrytv/internal/errors"
	"github.com/lbryio/lbrytv/internal/monitor"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var logger = monitor.NewModuleLogger("publish")

const (
	// fileFieldName refers to the POST field containing file upload
	fileFieldName = "file"
	// jsonRPCFieldName is a name of the POST field containing JSONRPC request accompanying the uploaded file
	jsonRPCFieldName = "json_payload"

	fileNameParam = "file_path"
)

// Handler has path to save uploads to
type Handler struct {
	UploadPath string
}

// Handle is where HTTP upload is handled and passed on to Publisher.
// It should be wrapped with users.Authenticator.Wrap before it can be used
// in a mux.Router.
func (h Handler) Handle(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	authResult := auth.FromRequest(r)
	if !proxy.EnsureAuthenticated(authResult, w) {
		return
	}
	if authResult.SDKAddress == "" {
		w.Write(proxy.NewInternalError(errors.Err("user does not have sdk address assigned")).JSON())
		logger.Log().Errorf("user %d does not have sdk address assigned", authResult.User().ID)
		return
	}

	f, err := h.saveFile(r, authResult.User().ID)
	if err != nil {
		logger.Log().Error(err)
		monitor.ErrorToSentry(err)
		w.Write(proxy.NewInternalError(err).JSON())
		return
	}
	defer func() {
		if err := os.Remove(f.Name()); err != nil {
			monitor.ErrorToSentry(err, map[string]string{"file_path": f.Name()})
		}
	}()

	res := publish(
		authResult.SDKAddress,
		f.Name(),
		authResult.User().ID,
		[]byte(r.FormValue(jsonRPCFieldName)),
	)

	w.Write(res)
}

func publish(sdkAddress, filename string, userID int, rawQuery []byte) []byte {
	c := proxy.NewCaller(sdkAddress, userID)
	c.Preprocessor = func(q *proxy.Query) {
		params := q.ParamsAsMap()
		params[fileNameParam] = filename
		q.Request.Params = params
	}
	return c.CallRaw(rawQuery)
}

// CanHandle checks if http.Request contains POSTed data in an accepted format.
// Supposed to be used in gorilla mux router MatcherFunc.
func (h Handler) CanHandle(r *http.Request, _ *mux.RouteMatch) bool {
	_, _, err := r.FormFile(fileFieldName)
	return !errors.Is(err, http.ErrMissingFile) && r.FormValue(jsonRPCFieldName) != ""
}

func (h Handler) saveFile(r *http.Request, userID int) (*os.File, error) {
	log := logger.WithFields(logrus.Fields{"user_id": userID})

	file, header, err := r.FormFile(fileFieldName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	f, err := h.createFile(userID, header.Filename)
	if err != nil {
		return nil, err
	}
	log.Infof("processing uploaded file %v", header.Filename)

	numWritten, err := io.Copy(f, file)
	if err != nil {
		return nil, err
	}
	log.Infof("saved uploaded file %v (%v bytes written)", f.Name(), numWritten)

	if err := f.Close(); err != nil {
		return nil, err
	}
	return f, nil
}

// createFile opens an empty file for writing inside the account's designated folder.
// The final file path looks like `/upload_path/{user_id}/{random}_filename.ext`,
// where `user_id` is user's ID and `random` is a random string generated by ioutil.
func (h Handler) createFile(userID int, origFilename string) (*os.File, error) {
	path := path.Join(h.UploadPath, fmt.Sprintf("%d", userID))
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return ioutil.TempFile(path, fmt.Sprintf("*_%s", origFilename))
}
