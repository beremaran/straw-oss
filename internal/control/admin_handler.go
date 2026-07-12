package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/beremaran/straw-oss/internal/config"
)

const adminItemsKey = "items"

var errSingleJSONValue = errors.New("request must contain one JSON value")

// AdminHandler exposes the deployment-scoped runtime administration surface.
type AdminHandler struct {
	service *AdminService
	auth    *AdminAuthenticator
}

// NewAdminHandler binds a service to its dedicated authenticator.
func NewAdminHandler(service *AdminService, auth *AdminAuthenticator) *AdminHandler {
	return &AdminHandler{service: service, auth: auth}
}

// Register adds every documented Admin/Config API and dashboard route.
func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/", h.dashboard)
	mux.HandleFunc("GET /api/v1/admin/config", h.getConfig)
	mux.HandleFunc("PUT /api/v1/admin/config", h.putConfig)
	mux.HandleFunc("GET /api/v1/admin/config/history", h.history)
	mux.HandleFunc("POST /api/v1/admin/config/rollback", h.rollback)
	mux.HandleFunc("GET /api/v1/admin/rollouts", h.rollout)
	mux.HandleFunc("GET /api/v1/admin/workers", h.workers)
	mux.HandleFunc("POST /api/v1/admin/workers/{worker_id}/{action}", h.workerAction)
	mux.HandleFunc("GET /api/v1/admin/requests", h.requests)
	mux.HandleFunc("DELETE /api/v1/admin/requests/{request_id}", h.cancelRequest)
}

func (h *AdminHandler) dashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, adminDashboardHTML)
}

func (h *AdminHandler) authorized(w http.ResponseWriter, r *http.Request) bool {
	if h.auth == nil || !h.auth.Authorize(r.Context(), r.Header.Get("Authorization")) {
		writeAdminError(w, http.StatusUnauthorized, "admin_auth_required")

		return false
	}

	return true
}

func (h *AdminHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	record, err := h.service.Current()
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())

		return
	}

	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", record.Revision))
	writeAdminJSON(w, http.StatusOK, record)
}

func (h *AdminHandler) putConfig(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	revision, ok := expectedRevision(w, r)
	if !ok {
		return
	}

	var snapshot config.Snapshot

	err := decodeAdminJSON(r, &snapshot)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())

		return
	}

	record, err := h.service.Update(revision, snapshot, r.Header.Get("X-Straw-Actor"), "update")
	writeMutation(w, record, err)
}

func (h *AdminHandler) history(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	records, err := h.service.History()
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())

		return
	}

	writeAdminJSON(w, http.StatusOK, map[string]any{adminItemsKey: records})
}

func (h *AdminHandler) rollback(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	revision, ok := expectedRevision(w, r)
	if !ok {
		return
	}

	var body struct {
		ConfigVersion uint64 `json:"config_version"`
	}

	err := decodeAdminJSON(r, &body)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())

		return
	}

	record, err := h.service.Rollback(revision, body.ConfigVersion, r.Header.Get("X-Straw-Actor"))
	writeMutation(w, record, err)
}

func (h *AdminHandler) rollout(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	writeAdminJSON(w, http.StatusOK, h.service.Rollout())
}

func (h *AdminHandler) workers(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	writeAdminJSON(w, http.StatusOK, map[string]any{adminItemsKey: h.service.Workers()})
}

func (h *AdminHandler) workerAction(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	revision, ok := expectedRevision(w, r)
	if !ok {
		return
	}

	record, err := h.service.SetWorker(revision, r.PathValue("worker_id"), r.PathValue("action"), r.Header.Get("X-Straw-Actor"))
	writeMutation(w, record, err)
}

func (h *AdminHandler) requests(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	writeAdminJSON(w, http.StatusOK, map[string]any{adminItemsKey: h.service.Requests(r.Context())})
}

func (h *AdminHandler) cancelRequest(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(w, r) {
		return
	}

	if !h.service.CancelRequest(r.Context(), r.PathValue("request_id")) {
		writeAdminError(w, http.StatusNotFound, "request_not_found")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func expectedRevision(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	raw := strings.Trim(r.Header.Get("If-Match"), "\"")
	if raw == "" {
		writeAdminError(w, http.StatusPreconditionRequired, "if_match_required")

		return 0, false
	}

	revision, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_if_match")

		return 0, false
	}

	return revision, true
}

func decodeAdminJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxHandlerBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(dst)
	if err != nil {
		return fmt.Errorf("decode administrative request: %w", err)
	}

	err = decoder.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errSingleJSONValue
	}

	return nil
}

func writeMutation(w http.ResponseWriter, record ConfigRecord, err error) {
	if errors.Is(err, ErrConfigConflict) {
		writeAdminError(w, http.StatusConflict, "revision_conflict")

		return
	}

	if errors.Is(err, config.ErrInvalidSnapshot) {
		writeAdminError(w, http.StatusUnprocessableEntity, err.Error())

		return
	}

	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())

		return
	}

	w.Header().Set("ETag", fmt.Sprintf("\"%d\"", record.Revision))
	writeAdminJSON(w, http.StatusOK, record)
}

func writeAdminJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encodeErr := json.NewEncoder(w).Encode(value)
	if encodeErr != nil {
		return
	}
}

func writeAdminError(w http.ResponseWriter, status int, message string) {
	writeAdminJSON(w, status, map[string]string{"error": message})
}

const adminDashboardHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Straw runtime administration</title><style>
:root{color-scheme:light dark;font:15px system-ui}body{max-width:1100px;margin:2rem auto;padding:0 1rem}header{display:flex;gap:.7rem;align-items:center;flex-wrap:wrap}input,button,textarea{font:inherit;padding:.55rem}input{min-width:20rem}button{cursor:pointer}textarea{width:100%;min-height:25rem;font-family:ui-monospace,monospace}section{margin:2rem 0;border-top:1px solid #888;padding-top:1rem}.row{display:flex;gap:.5rem;align-items:center;flex-wrap:wrap}.card{padding:.8rem;margin:.5rem 0;border:1px solid #888;border-radius:.4rem}pre{white-space:pre-wrap}.error{color:#e44}</style></head><body>
<header><h1>Straw runtime administration</h1><input id="token" type="password" placeholder="Admin bearer token"><button onclick="loadAll()">Connect</button></header><p id="message"></p>
<section><h2>Configuration</h2><div class="row"><button onclick="saveConfig()">Validate and activate</button><span id="version"></span></div><textarea id="config"></textarea></section>
<section><h2>Workers</h2><div id="workers"></div></section><section><h2>In-flight requests</h2><div id="requests"></div></section>
<section><h2>Rollout</h2><pre id="rollout"></pre></section><section><h2>Audit and rollback</h2><div id="history"></div></section>
<script>
let revision=0;const token=()=>document.querySelector('#token').value;async function api(path,options={}){options.headers={...(options.headers||{}),Authorization:'Bearer '+token(),'X-Straw-Actor':'dashboard'};const r=await fetch(path,options);if(!r.ok){let x=await r.text();throw new Error(r.status+' '+x)}return r.status===204?null:r.json()}
async function loadAll(){try{let c=await api('/api/v1/admin/config');revision=c.revision;version.textContent='version '+c.snapshot.config_version+' · revision '+revision;config.value=JSON.stringify(c.snapshot,null,2);let [w,q,o,h]=await Promise.all([api('/api/v1/admin/workers'),api('/api/v1/admin/requests'),api('/api/v1/admin/rollouts'),api('/api/v1/admin/config/history')]);workers.innerHTML=w.items.map(x=>card(x,['drain','undrain','disable','enable'].map(a=>'<button onclick="worker(\''+x.worker_id+'\',\''+a+'\')">'+a+'</button>').join(''))).join('')||'<p>No workers registered.</p>';requests.innerHTML=q.items.map(x=>card(x,'<button onclick="cancelReq(\''+x.request_id+'\')">cancel safely</button>')).join('')||'<p>No active requests.</p>';rollout.textContent=JSON.stringify(o,null,2);history.innerHTML=h.items.map(x=>card({version:x.snapshot.config_version,revision:x.revision,actor:x.actor,action:x.action,created_at:x.created_at},'<button onclick="rollback('+x.snapshot.config_version+')">rollback</button>')).join('');message.textContent='';}catch(e){message.className='error';message.textContent=e.message}}
function card(x,buttons){return '<div class="card"><pre>'+esc(JSON.stringify(x,null,2))+'</pre><div>'+buttons+'</div></div>'}function esc(s){return s.replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}
async function mutate(path,method='POST',body){await api(path,{method,headers:{'If-Match':String(revision),'Content-Type':'application/json'},body:body&&JSON.stringify(body)});await loadAll()}
async function saveConfig(){try{await mutate('/api/v1/admin/config','PUT',JSON.parse(config.value))}catch(e){message.className='error';message.textContent=e.message}}
async function worker(id,a){await mutate('/api/v1/admin/workers/'+encodeURIComponent(id)+'/'+a)}async function rollback(v){await mutate('/api/v1/admin/config/rollback','POST',{config_version:v})}async function cancelReq(id){await api('/api/v1/admin/requests/'+encodeURIComponent(id),{method:'DELETE'});await loadAll()}
</script></body></html>`
