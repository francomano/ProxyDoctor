package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins/localproxy"
)

// localProxyState holds the currently running local forward proxy.
type localProxyState struct {
	mu        sync.Mutex
	plugin    *localproxy.LocalProxyPlugin
	proxy     string
	proxyType string
}

var localProxy = &localProxyState{}

type localProxyStatus struct {
	Running   bool   `json:"running"`
	Address   string `json:"address"`
	Proxy     string `json:"proxy"`
	ProxyType string `json:"proxy_type"`
}

type localProxyStartRequest struct {
	Proxy     string `json:"proxy"`
	ProxyType string `json:"proxy_type"`
}

func localProxyStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	localProxy.mu.Lock()
	defer localProxy.mu.Unlock()

	st := localProxyStatus{}
	if localProxy.plugin != nil {
		st.Running = true
		st.Address = localProxy.plugin.Addr()
		st.Proxy = localProxy.proxy
		st.ProxyType = localProxy.proxyType
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

func localProxyStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body localProxyStartRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	localProxy.mu.Lock()
	defer localProxy.mu.Unlock()

	if localProxy.plugin != nil {
		writeJSONError(w, http.StatusConflict, "local proxy is already running on "+localProxy.plugin.Addr())
		return
	}

	p := localproxy.New()
	if err := p.Init(&plugin.Context{
		Config: map[string]interface{}{
			"proxy":      body.Proxy,
			"proxy_type": body.ProxyType,
		},
	}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start local proxy: %v", err))
		return
	}

	localProxy.plugin = p
	localProxy.proxy = body.Proxy
	localProxy.proxyType = body.ProxyType

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(localProxyStatus{Running: true, Address: p.Addr(), Proxy: body.Proxy, ProxyType: body.ProxyType})
}

func localProxyStopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	localProxy.mu.Lock()
	defer localProxy.mu.Unlock()

	if localProxy.plugin == nil {
		writeJSONError(w, http.StatusConflict, "local proxy is not running")
		return
	}

	if err := localProxy.plugin.Shutdown(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to stop local proxy: %v", err))
		return
	}
	localProxy.plugin = nil

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(localProxyStatus{Running: false})
}
