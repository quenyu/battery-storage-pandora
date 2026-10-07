package handler

import (
	"battery-storage-pandora/internal/model"
	"context"
	"net/http"
)

type createUserRequest struct {
	Name    *string `json:"name"`
	Barcode *string `json:"barcode"`
}

type patchUserRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body createUserRequest
	if err := decodeRequest(w, r, &body, "name barcode"); err != nil {
		writeError(w, err)
		return
	}
	if err := requiredText(body.Name, body.Barcode); err != nil {
		writeError(w, err)
		return
	}

	input := model.CreateUserInput{Name: *body.Name, Barcode: *body.Barcode}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.CreateUser(ctx, input)
	})
}

func (s *Server) patchUser(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "user_id")
	if err != nil {
		writeError(w, err)
		return
	}
	var body patchUserRequest
	if err := decodeRequest(w, r, &body, "name is_active"); err != nil {
		writeError(w, err)
		return
	}
	if err := optionalText(body.Name); err != nil {
		writeError(w, err)
		return
	}

	input := model.PatchUserInput{Name: body.Name, IsActive: body.IsActive}
	executeCommand(w, r, func(ctx context.Context) (model.CommandResponse, error) {
		return s.service.PatchUser(ctx, userID, input)
	})
}
