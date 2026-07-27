package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/francomano/proxydoctor/core/adapters"
	checkspkg "github.com/francomano/proxydoctor/core/checks"
	"github.com/francomano/proxydoctor/core/engine"
	"github.com/francomano/proxydoctor/core/plugin"
	"github.com/francomano/proxydoctor/core/plugins"
	"github.com/francomano/proxydoctor/core/utils"
)

// newRegistry builds a check registry with every built-in check and all
// available plugins registered.
func newRegistry() *engine.CheckRegistry {
	registry := engine.NewCheckRegistry()
	if err := checkspkg.RegisterDefaults(registry); err != nil {
		log.Fatalf("failed to register checks: %v", err)
	}

	mgr := plugin.NewManager()
	ctx := &plugin.Context{Registry: registry, Config: map[string]interface{}{}}
	if err := plugins.Load([]string{"all"}, mgr, ctx); err != nil {
		log.Printf("warning: failed to load some plugins: %v", err)
	}

	return registry
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/api/checks", checksHandler)
	mux.HandleFunc("/api/diagnose", diagnoseHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	log.Println("ProxyDoctor server starting on :8080")
	log.Println("  GUI:          http://localhost:8080/")
	log.Println("  API checks:   GET  /api/checks")
	log.Println("  API diagnose: POST /api/diagnose")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// indexHandler serves the single-page GUI.
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

// checkInfo is the JSON-friendly view of a registered check.
type checkInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

func checksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	registry := newRegistry()
	checks := registry.ListChecks()

	infos := make([]checkInfo, 0, len(checks))
	for _, c := range checks {
		infos = append(infos, checkInfo{
			ID:          c.ID(),
			Name:        c.Name(),
			Description: c.Description(),
			Category:    string(c.Category()),
			DependsOn:   c.DependsOn(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infos)
}

// diagnoseRequestBody is the JSON body accepted by POST /api/diagnose.
type diagnoseRequestBody struct {
	URL       string `json:"url"`
	Proxy     string `json:"proxy"`      // e.g. "http://host:port", empty = direct
	ProxyType string `json:"proxy_type"` // "auto", "http", "https", "socks4", "socks5"
}

func diagnoseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var body diagnoseRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if body.URL == "" {
		writeJSONError(w, http.StatusBadRequest, "\"url\" is required")
		return
	}

	proxyConfig, err := utils.ParseProxyConfig(body.Proxy, body.ProxyType)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Same wiring as cmd/cli/commands/diagnose.go: registry -> adapter factory -> orchestrator.
	registry := newRegistry()
	adapterFactory := adapters.NewAdapterFactory()
	orchestrator := engine.NewDiagnosisOrchestrator(registry, adapterFactory, 4)

	diagRequest := engine.DiagnosisRequest{
		URL:         body.URL,
		ProxyConfig: proxyConfig,
		Timeout:     30 * time.Second,
	}

	report, err := orchestrator.Execute(diagRequest)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "diagnosis failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<title>ProxyDoctor</title>
<meta name="viewport" content="width=device-width, initial-scale=1" />
<style>
  :root { color-scheme: dark; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    max-width: 720px;
    margin: 40px auto;
    padding: 0 20px;
    background: #0f1115;
    color: #e6e6e6;
  }
  h1 { font-size: 1.4rem; margin-bottom: 4px; }
  p.sub { color: #9aa0a6; margin-top: 0; }
  form {
    background: #1a1d24;
    border: 1px solid #2a2e37;
    border-radius: 10px;
    padding: 20px;
    display: grid;
    gap: 12px;
  }
  label { font-size: 0.85rem; color: #b6bac2; display: block; margin-bottom: 4px; }
  input, select {
    width: 100%;
    box-sizing: border-box;
    background: #0f1115;
    border: 1px solid #2a2e37;
    color: #e6e6e6;
    padding: 8px 10px;
    border-radius: 6px;
    font-size: 0.95rem;
  }
  .row { display: grid; grid-template-columns: 2fr 1fr; gap: 12px; }
  button {
    background: #3b82f6;
    color: white;
    border: none;
    padding: 10px 16px;
    border-radius: 6px;
    font-size: 0.95rem;
    cursor: pointer;
  }
  button:disabled { opacity: 0.6; cursor: default; }
  #results { margin-top: 24px; }
  .card {
    background: #1a1d24;
    border: 1px solid #2a2e37;
    border-radius: 10px;
    padding: 16px;
    margin-bottom: 12px;
  }
  .card.passed { border-left: 4px solid #22c55e; }
  .card.failed { border-left: 4px solid #ef4444; }
  .card.error  { border-left: 4px solid #f59e0b; }
  .badge {
    display: inline-block;
    font-size: 0.75rem;
    padding: 2px 8px;
    border-radius: 999px;
    background: #2a2e37;
    color: #b6bac2;
    margin-right: 6px;
  }
  pre {
    background: #0f1115;
    border: 1px solid #2a2e37;
    border-radius: 6px;
    padding: 10px;
    overflow-x: auto;
    font-size: 0.8rem;
  }
  #error { color: #ef4444; margin-top: 12px; white-space: pre-wrap; }
  #summary { color: #9aa0a6; font-size: 0.85rem; margin-top: 14px; }
</style>
</head>
<body>
  <h1>🩺 ProxyDoctor</h1>
  <p class="sub">Run a diagnosis directly from the browser, wired to the real core engine.</p>

  <form id="diagnose-form">
    <div>
      <label for="url">URL to diagnose</label>
      <input id="url" name="url" type="text" placeholder="https://example.com" required />
    </div>
    <div class="row">
      <div>
        <label for="proxy">Proxy (optional)</label>
        <input id="proxy" name="proxy" type="text" placeholder="host:port or scheme://host:port" />
      </div>
      <div>
        <label for="proxy_type">Proxy type</label>
        <select id="proxy_type" name="proxy_type">
          <option value="auto">auto (from scheme)</option>
          <option value="http">HTTP</option>
          <option value="https">HTTPS</option>
          <option value="socks4">SOCKS4</option>
          <option value="socks5">SOCKS5</option>
        </select>
      </div>
    </div>
    <p style="font-size:0.75rem;color:#6b7280;margin:0;">
      Proxy URL examples: <code>socks5://77.245.76.107:1080</code>, <code>http://proxy:3128</code>, or just <code>host:port</code> and select the type above.
    </p>
    <button type="submit" id="submit-btn">Run diagnosis</button>
  </form>

  <div id="error"></div>
  <div id="results"></div>

  <script>
    const form = document.getElementById('diagnose-form');
    const resultsEl = document.getElementById('results');
    const errorEl = document.getElementById('error');
    const submitBtn = document.getElementById('submit-btn');

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      errorEl.textContent = '';
      resultsEl.innerHTML = '';
      submitBtn.disabled = true;
      submitBtn.textContent = 'Running…';

      const payload = {
        url: document.getElementById('url').value.trim(),
        proxy: document.getElementById('proxy').value.trim(),
        proxy_type: document.getElementById('proxy_type').value,
      };

      try {
        const resp = await fetch('/api/diagnose', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        const data = await resp.json();

        if (!resp.ok) {
          errorEl.textContent = data.error || ('HTTP ' + resp.status);
          return;
        }

        renderReport(data);
      } catch (err) {
        errorEl.textContent = String(err);
      } finally {
        submitBtn.disabled = false;
        submitBtn.textContent = 'Run diagnosis';
      }
    });

    function renderReport(report) {
      const results = report.results || [];
      if (results.length === 0) {
        resultsEl.innerHTML = '<p>No checks executed.</p>';
        return;
      }

      results.forEach((r) => {
        const card = document.createElement('div');
        card.className = 'card ' + (r.status || '');

        const icon = r.status === 'passed' ? '✅' : (r.status === 'error' ? '⚠️' : '❌');

        card.innerHTML =
          '<div><strong>' + icon + ' ' + r.id + '</strong></div>' +
          '<div style="margin:6px 0;">' +
            '<span class="badge">' + r.status + '</span>' +
            '<span class="badge">' + r.severity + '</span>' +
            '<span class="badge">' + Math.round((r.confidence || 0) * 100) + '% confidence</span>' +
          '</div>' +
          '<div>' + (r.explanation || '') + '</div>' +
          (r.evidence && Object.keys(r.evidence).length
            ? '<pre>' + JSON.stringify(r.evidence, null, 2) + '</pre>'
            : '');

        resultsEl.appendChild(card);
      });

      const summary = document.createElement('div');
      summary.id = 'summary';
      summary.textContent =
        'Checks executed: ' + report.checks_executed +
        ' | Failed: ' + report.checks_failed +
        ' | Critical: ' + report.critical_findings +
        ' | Total time: ' + (report.execution_time / 1e6).toFixed(1) + 'ms';
      resultsEl.appendChild(summary);
    }
  </script>
</body>
</html>
`
