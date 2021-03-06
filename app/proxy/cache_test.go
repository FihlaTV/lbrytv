package proxy

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ybbus/jsonrpc"
)

func TestCache(t *testing.T) {
	var (
		response jsonrpc.RPCResponse
		query    jsonrpc.RPCRequest
	)

	// params := map[string]interface{}{"urls": []string{"one", "two", "three"}}
	rawQuery := `{"jsonrpc":"2.0","method":"resolve","params":{"urls":["one", "two", "three"]},"id":1555013448981}`
	err := json.Unmarshal([]byte(rawQuery), &query)
	if err != nil {
		t.Fatal(err)
	}

	absPath, _ := filepath.Abs("./testdata/resolve_response.json")
	rawJSON, err := ioutil.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(rawJSON, &response)
	if err != nil {
		t.Fatal(err)
	}

	globalCache.flush()
	assert.Nil(t, globalCache.Retrieve("resolve", query.Params))
	globalCache.Save("resolve", query.Params, response.Result)
	assert.Equal(t, 1, globalCache.Count())
	assert.Equal(t, response.Result, globalCache.Retrieve("resolve", query.Params))
}

func TestCacheGetKey(t *testing.T) {
	globalCache.flush()
	key, err := globalCache.getKey("resolve", map[string]interface{}{"urls": "one"})
	assert.Equal(t, "resolve|3600a4eed065d3ae3dd503cca56ce56ae6bd4778047fa1b17c999301681d3a1d", key)
	assert.NoError(t, err)

	key, err = globalCache.getKey("wallet_balance", nil)
	assert.Equal(t, "wallet_balance|nil", key)
	assert.NoError(t, err)
}
