package api

import (
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/node"
)

type nodeTypeHandler struct {
	registry *node.NodeRegistry
}

func (h *nodeTypeHandler) list(w http.ResponseWriter, _ *http.Request) {
	metas := h.registry.ListAll()
	writeJSON(w, http.StatusOK, map[string]any{"node_types": metas})
}
