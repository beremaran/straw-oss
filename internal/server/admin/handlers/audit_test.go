package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	"github.com/beremaran/straw/internal/server/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAuditHandler_HandleListEvents(t *testing.T) {
	mockRepo := new(MockManagementAuditRepo)
	mockIdentity := new(MockIdentityRepo)
	handler := NewAuditHandler(mockRepo, mockIdentity)

	t.Run("Returns Events Successfully", func(t *testing.T) {
		events := []*domain.ManagementAuditEvent{
			{ID: 1, Action: "create", EntityType: "api_key"},
		}
		mockRepo.On("ListEvents", mock.Anything, mock.Anything).Return(events, 1, nil).Once()
		
		req, _ := http.NewRequest(http.MethodGet, "/management/audit/events?limit=10&page=1", nil)
		// Act as owner to get bodies
		ctx := middleware.ContextWithActor(req.Context(), middleware.LegacyAdminActor())
		req = req.WithContext(ctx)
		
		rr := httptest.NewRecorder()
		handler.HandleListEvents(rr, req)
		
		assert.Equal(t, http.StatusOK, rr.Code)
		
		var resp dto.ListAuditEventsResponse
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		
		assert.Equal(t, 1, resp.Total)
		assert.Len(t, resp.Data, 1)
		mockRepo.AssertExpectations(t)
	})
}

func TestAuditHandler_HandleGetEvent(t *testing.T) {
	mockRepo := new(MockManagementAuditRepo)
	mockIdentity := new(MockIdentityRepo)
	handler := NewAuditHandler(mockRepo, mockIdentity)

	t.Run("Redacts Body for Non-Owners", func(t *testing.T) {
		event := &domain.ManagementAuditEvent{
			ID: 1, Action: "create", EntityType: "api_key",
			OldValue: "secret_old", NewValue: "secret_new",
		}
		mockRepo.On("GetEventByID", mock.Anything, int64(1)).Return(event, nil).Once()
		
		req, _ := http.NewRequest(http.MethodGet, "/management/audit/events/1", nil)
		req.SetPathValue("id", "1")
		
		// Normal user without owner role
		actor := middleware.Actor{Type: middleware.ActorTypeUser, ID: "user-1"}
		ctx := middleware.ContextWithActor(req.Context(), actor)
		req = req.WithContext(ctx)
		
		mockIdentity.On("ListUserRoles", mock.Anything, "user-1").Return([]domain.AdminRole{{Name: "Operator"}}, nil).Once()
		
		rr := httptest.NewRecorder()
		handler.HandleGetEvent(rr, req)
		
		assert.Equal(t, http.StatusOK, rr.Code)
		
		var resp dto.AuditEvent
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		
		assert.Equal(t, int64(1), resp.ID)
		assert.Nil(t, resp.OldValue)
		assert.Nil(t, resp.NewValue)
		
		mockRepo.AssertExpectations(t)
		mockIdentity.AssertExpectations(t)
	})
}

func TestAuditHandler_HandleExport(t *testing.T) {
	mockRepo := new(MockManagementAuditRepo)
	mockIdentity := new(MockIdentityRepo)
	handler := NewAuditHandler(mockRepo, mockIdentity)

	t.Run("Rejects Missing Date Range", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/management/audit/export", nil)
		rr := httptest.NewRecorder()
		
		handler.HandleExport(rr, req)
		
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
	
	t.Run("Rejects Excessive Date Range", func(t *testing.T) {
		start := time.Now().Add(-32 * 24 * time.Hour).Format(time.RFC3339)
		end := time.Now().Format(time.RFC3339)
		
		req, _ := http.NewRequest(http.MethodGet, "/management/audit/export?start_date="+start+"&end_date="+end, nil)
		rr := httptest.NewRecorder()
		
		handler.HandleExport(rr, req)
		
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
