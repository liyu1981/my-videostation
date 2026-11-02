package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/liyu1981/myvideostation/internal/auth"
	"github.com/liyu1981/myvideostation/internal/store"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type createUserInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in createUserInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if err := auth.ValidateUsername(in.Username); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if in.Role == "" {
		in.Role = store.RoleUser
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleUser {
		writeError(w, http.StatusBadRequest, "bad_request", "role must be admin or user")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	u, err := s.store.CreateUser(r.Context(), in.Username, in.DisplayName, hash, in.Role)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "user.create", "user:"+strconv.FormatInt(u.ID, 10), u.Username+" ("+u.Role+")")
	writeJSON(w, http.StatusCreated, u)
}

type updateUserInput struct {
	DisplayName   *string `json:"displayName"`
	Role          *string `json:"role"`
	Active        *bool   `json:"active"`
	Password      *string `json:"password"`
	RatingCeiling *string `json:"ratingCeiling"`
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid user id")
		return
	}
	var in updateUserInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	current := userFrom(r)
	if current != nil && current.ID == id {
		if in.Role != nil && *in.Role != store.RoleAdmin {
			writeError(w, http.StatusBadRequest, "bad_request", "you cannot remove your own admin role")
			return
		}
		if in.Active != nil && !*in.Active {
			writeError(w, http.StatusBadRequest, "bad_request", "you cannot deactivate your own account")
			return
		}
	}
	if in.RatingCeiling != nil && !validRatingCeiling(*in.RatingCeiling) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid rating ceiling")
		return
	}
	var passwordHash *string
	if in.Password != nil && *in.Password != "" {
		if err := auth.ValidatePassword(*in.Password); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		hash, err := auth.HashPassword(*in.Password)
		if err != nil {
			s.fail(w, err)
			return
		}
		passwordHash = &hash
	}
	u, err := s.store.UpdateUser(r.Context(), id, in.DisplayName, in.Role, in.Active, passwordHash, in.RatingCeiling)
	if err != nil {
		s.fail(w, err)
		return
	}
	action := "user.update"
	if passwordHash != nil {
		action = "user.password_reset"
	}
	s.audit(r, action, "user:"+strconv.FormatInt(u.ID, 10), u.Username+" ("+u.Role+")")
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid user id")
		return
	}
	if current := userFrom(r); current != nil && current.ID == id {
		writeError(w, http.StatusBadRequest, "bad_request", "you cannot delete your own account")
		return
	}
	u, _ := s.store.GetUserByID(r.Context(), id)
	if err := s.store.DeleteUser(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	if u != nil {
		s.audit(r, "user.delete", "user:"+strconv.FormatInt(id, 10), u.Username)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUserLibraries(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid user id")
		return
	}
	if _, err := s.store.GetUserByID(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	libs, err := s.store.UserLibraryAccess(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"libraries": libs})
		return
	}
	var in struct {
		LibraryIDs   []int64 `json:"libraryIds"`
		Unrestricted bool    `json:"unrestricted"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return
	}
	if in.Unrestricted {
		if err := s.store.SetUserLibraryUnrestricted(r.Context(), id); err != nil {
			s.fail(w, err)
			return
		}
		s.audit(r, "user.library_permissions", "user:"+strconv.FormatInt(id, 10), "library access unrestricted")
	} else {
		if err := s.store.SetUserLibraryPermissions(r.Context(), id, in.LibraryIDs); err != nil {
			s.fail(w, err)
			return
		}
		s.audit(r, "user.library_permissions", "user:"+strconv.FormatInt(id, 10), "library access updated")
	}
	libs, err = s.store.UserLibraryAccess(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": libs})
}

func validRatingCeiling(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "", "G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA":
		return true
	}
	return false
}
