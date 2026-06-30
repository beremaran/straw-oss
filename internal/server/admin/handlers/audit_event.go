package handlers

import (
	"net/http"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
)

func recordAuditEvent(r *http.Request, repo domain.ManagementAuditRepository, action, entityType, entityID string, oldValue, newValue any) {
	if repo == nil {
		return
	}

	event := middleware.NewAuditEvent(r, action, entityType, entityID, oldValue, newValue)
	_ = repo.Create(r.Context(), event)
}
