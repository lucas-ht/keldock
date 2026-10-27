package github

import "net/http"

func (service Service) setup(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		Purpose         string `json:"purpose"`
	}
	if !decode(w, r, &input) {
		return
	}
	setup, err := service.Integration.BeginSetup(r.Context(), principal, input.ExpectedVersion, input.Purpose)
	if err != nil {
		failure(w, err)
		return
	}
	write(w, 200, setup)
}

func (service Service) complete(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		State   string `json:"state"`
		Purpose string `json:"purpose"`
		Code    string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := service.Integration.CompleteSetup(r.Context(), principal, input.State, input.Purpose, input.Code); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "configured"})
}

func (service Service) webhook(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := service.Integration.ConfigureWebhook(r.Context(), principal, input.ExpectedVersion); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "configured"})
}

func (service Service) retry(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.administrator(w, r, true)
	if !ok {
		return
	}
	var input struct {
		ExpectedVersion int64 `json:"expectedVersion"`
	}
	if !decode(w, r, &input) {
		return
	}
	if err := service.Integration.Repository.RetryWork(r.Context(), principal, input.ExpectedVersion); err != nil {
		failure(w, err)
		return
	}
	write(w, 200, map[string]string{"state": "queued"})
}
