package handlers

import (
	"net/http"
	"strings"

	"github.com/beremaran/straw/internal/server/helper"
)

func writeConflictOrServerError(w http.ResponseWriter, err error, conflictMsg, serverMsg string) {
	if isUniqueConstraintError(err) {
		helper.WriteError(w, http.StatusConflict, conflictMsg)

		return
	}

	helper.WriteError(w, http.StatusInternalServerError, serverMsg)
}

func isUniqueConstraintError(err error) bool {
	msg := err.Error()

	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
