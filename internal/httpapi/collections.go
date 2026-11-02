package httpapi

import (
	"net/http"
	"strconv"

	"github.com/liyu1981/myvideostation/internal/store"
)

func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListCollections(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": list})
}

type collectionInput struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	var in collectionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	c, err := s.store.CreateCollection(r.Context(), in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleRenameCollection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid collection id")
		return
	}
	var in collectionInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if err := s.store.RenameCollection(r.Context(), id, in.Name); err != nil {
		s.fail(w, err)
		return
	}
	c, err := s.store.GetCollection(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid collection id")
		return
	}
	if err := s.store.DeleteCollection(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCollectionItems(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid collection id")
		return
	}
	if _, err := s.store.GetCollection(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	items, total, err := s.store.ListCollectionItems(r.Context(), id, intQuery(r, "page", 1), intQuery(r, "perPage", 60))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.itemDTOs(items),
		"total": total,
		"page":  intQuery(r, "page", 1),
	})
}

type collectionItemsInput struct {
	ItemIDs []int64 `json:"itemIds"`
}

func (s *Server) handleAddCollectionItems(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid collection id")
		return
	}
	var in collectionItemsInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if _, err := s.store.GetCollection(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.AddToCollection(r.Context(), id, in.ItemIDs); err != nil {
		s.fail(w, err)
		return
	}
	c, err := s.store.GetCollection(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleRemoveCollectionItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid collection id")
		return
	}
	itemID, err := strconv.ParseInt(r.PathValue("itemId"), 10, 64)
	if err != nil || itemID <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid item id")
		return
	}
	if err := s.store.RemoveFromCollection(r.Context(), id, itemID); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type batchInput struct {
	Action       string  `json:"action"`
	ItemIDs      []int64 `json:"itemIds"`
	CollectionID int64   `json:"collectionId"`
}

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var in batchInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if len(in.ItemIDs) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "itemIds must not be empty")
		return
	}
	if len(in.ItemIDs) > 1000 {
		writeError(w, http.StatusBadRequest, "bad_request", "too many items in one batch (max 1000)")
		return
	}
	switch in.Action {
	case "watched":
		if err := s.store.MarkPlayed(r.Context(), in.ItemIDs, true); err != nil {
			s.fail(w, err)
			return
		}
	case "unwatched":
		if err := s.store.MarkPlayed(r.Context(), in.ItemIDs, false); err != nil {
			s.fail(w, err)
			return
		}
	case "add_collection":
		if in.CollectionID <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "collectionId is required")
			return
		}
		if err := s.store.AddToCollection(r.Context(), in.CollectionID, in.ItemIDs); err != nil {
			s.fail(w, err)
			return
		}
	case "remove_collection":
		if in.CollectionID <= 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "collectionId is required")
			return
		}
		for _, id := range in.ItemIDs {
			if err := s.store.RemoveFromCollection(r.Context(), in.CollectionID, id); err != nil {
				s.fail(w, err)
				return
			}
		}
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "unknown action: "+in.Action)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "applied": len(in.ItemIDs)})
}

func (s *Server) handleFacets(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	lib, err := s.store.GetLibrary(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = store.KindMovie
		if lib.Type == "tv" {
			kind = store.KindShow
		}
	}
	facets, err := s.store.Facets(r.Context(), id, kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid library id")
		return
	}
	if _, err := s.store.GetLibrary(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	folders, err := s.store.Folders(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": folders})
}
