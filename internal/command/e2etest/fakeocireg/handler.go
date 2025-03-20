// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package fakeocireg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	ociDigest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	orasOCI "oras.land/oras-go/v2/content/oci"
	orasErrs "oras.land/oras-go/v2/errdef"
)

type registryHandler struct {
	stores map[string]*orasOCI.ReadOnlyStore
}

var _ http.Handler = registryHandler{}

// ServeHTTP implements http.Handler.
func (r registryHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Printf("[INFO] fakeocireg: incoming request %s %s", req.Method, req.URL)
	if req.Method != "GET" && req.Method != "HEAD" {
		// This is a read-only implementation, so we don't support any other methods.
		resp.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	repoName, noun, arg, ok := parseReqPath(req.URL.Path)
	if !ok {
		log.Printf("[ERROR] fakeocireg: invalid request path")
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	if repoName == "" {
		// This is just a "does this server even support the protocol?" discovery request.
		log.Printf("[INFO] fakeocireg: protocol discovery succeeds")
		resp.WriteHeader(http.StatusOK)
		return
	}
	store, ok := r.stores[repoName]
	if !ok {
		log.Printf("[ERROR] fakeocireg: no repository named %q", repoName)
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	ctx := req.Context()
	repoHandler := &repositoryHandler{
		repoName: repoName,
		store:    store,
		resp:     resp,
		req:      req,
	}
	repoHandler.serve(ctx, noun, arg)
}

type repositoryHandler struct {
	repoName string
	store    *orasOCI.ReadOnlyStore
	resp     http.ResponseWriter
	req      *http.Request
}

func (h *repositoryHandler) serve(ctx context.Context, noun, arg string) {
	switch noun {
	case "manifests":
		h.serveManifest(ctx, arg)
	case "blobs":
		h.serveBlob(ctx, arg)
	case "tags":
		if arg != "list" {
			h.resp.WriteHeader(http.StatusNotFound)
			return
		}
		h.serveTagList(ctx)
	default:
		h.resp.WriteHeader(http.StatusNotFound)
	}
}

func (h *repositoryHandler) wantResponseBody() bool {
	return h.req.Method == "GET"
}

func (h *repositoryHandler) serveManifest(ctx context.Context, reference string) {
	desc, err := h.store.Resolve(ctx, reference)
	if err != nil {
		h.handleRequestError(ctx, err)
		return
	}
	h.serveManifestOrBlob(ctx, desc)
}

func (h *repositoryHandler) serveBlob(ctx context.Context, reference string) {
	// When requesting blobs, the reference is required to be a digest rather
	// than a tag. Otherwise, this is much the same as serving a manifest.
	digest, err := ociDigest.Parse(reference)
	if err != nil {
		h.resp.WriteHeader(http.StatusNotFound)
		return
	}
	desc, err := h.store.Resolve(ctx, digest.String())
	if err != nil {
		h.handleRequestError(ctx, err)
		return
	}
	h.serveManifestOrBlob(ctx, desc)
}

func (h *repositoryHandler) serveManifestOrBlob(ctx context.Context, desc ociv1.Descriptor) {
	reader, err := h.store.Fetch(ctx, desc)
	if err != nil {
		h.handleRequestError(ctx, err)
		return
	}
	defer reader.Close()
	h.resp.Header().Set("Content-Type", desc.MediaType)
	h.resp.Header().Set("Content-Length", strconv.FormatInt(desc.Size, 10))
	h.resp.Header().Set("Docker-Content-Digest", desc.Digest.String())
	h.resp.WriteHeader(http.StatusOK)
	if h.wantResponseBody() {
		// If this fails then there isn't really anything we can do about it;
		// presumably the client disconnected without waiting for all the data.
		_, _ = io.CopyN(h.resp, reader, desc.Size)
	}
}

func (h *repositoryHandler) serveTagList(ctx context.Context) {
	var limit = math.MaxInt
	var last = ""
	query := h.req.URL.Query()
	if query.Has("last") {
		last = query.Get("last")
	}
	if query.Has("n") {
		n, err := strconv.Atoi(query.Get("n"))
		if err != nil { // (we'll just silently ignore an invalid n)
			h.handleRequestError(ctx, err)
			return
		}
		limit = n
	}

	type ResponseBody struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	respBody := ResponseBody{
		Name: h.repoName,
	}

	// Since we know we're only going to be working with contrived content
	// provided in test fixtures, we'll just buffer the answer to this
	// into memory without worry.
	err := h.store.Tags(ctx, last, func(tags []string) error {
		respBody.Tags = append(respBody.Tags, tags...)
		return nil
	})
	if err != nil {
		log.Printf("[ERROR] fakeocireg: %s", err)
		h.resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(respBody.Tags) > limit {
		respBody.Tags = respBody.Tags[:limit]
	}
	respBodyRaw, err := json.MarshalIndent(&respBody, "", "  ")
	if err != nil {
		log.Printf("[ERROR] fakeocireg: %s", err)
		h.resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.resp.Header().Set("Content-Type", "application/json")
	h.resp.Header().Set("Content-Length", strconv.Itoa(len(respBodyRaw)))
	h.resp.WriteHeader(http.StatusOK)
	// Intentionally ignoring error because it should only happen if
	// the client disconnects before reading everything, or similar.
	_, _ = h.resp.Write(respBodyRaw)
}

func (h *repositoryHandler) handleRequestError(_ context.Context, err error) {
	if err == nil {
		panic("call handleError only with a non-nil error")
	}
	log.Printf("[ERROR] fakeocireg: %s", err)
	if errors.Is(err, orasErrs.ErrNotFound) {
		h.resp.WriteHeader(http.StatusNotFound)
	}
	h.resp.WriteHeader(http.StatusBadRequest)
}

func parseReqPath(reqPath string) (repoName, noun, arg string, ok bool) {
	// The OCI distribution protocol uses an interesting path scheme
	// where the first slash-delimited segment is always "v2",
	// the final two segments specify the repository content to interact with,
	// and everything in between is the repository name.
	// Therefore we need to do some light parsing here to extract
	// those different parts.
	const protocolPrefix = "/v2/"
	if !strings.HasPrefix(reqPath, protocolPrefix) {
		return "", "", "", false // not valid at all
	}
	if reqPath == protocolPrefix {
		// Requesting _just_ the prefix is valid for a client to
		// discover if a server implements the Distribution protocol.
		return "", "", "", true
	}
	reqPath = reqPath[len(protocolPrefix):]    // don't need to worry about the prefix anymore
	reqPath = strings.TrimSuffix(reqPath, "/") // ignore a trailing slash, if present

	slashIdx := strings.LastIndexByte(reqPath, '/')
	if slashIdx == -1 {
		return "", "", "", false
	}
	reqPath, arg = reqPath[:slashIdx], reqPath[slashIdx+1:]
	slashIdx = strings.LastIndexByte(reqPath, '/')
	if slashIdx == -1 {
		return "", "", "", false
	}
	repoName, noun = reqPath[:slashIdx], reqPath[slashIdx+1:]

	return repoName, noun, arg, true
}
