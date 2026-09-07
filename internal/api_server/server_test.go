package apiserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func validationRouter() http.Handler {
	swagger, err := v1alpha1.GetSwagger()
	Expect(err).NotTo(HaveOccurred())
	validationSwagger := *swagger

	router := chi.NewRouter()
	router.Use(nethttpmiddleware.OapiRequestValidatorWithOptions(&validationSwagger, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		SilenceServersWarning: true,
		ErrorHandler:          openAPIValidationErrorHandler,
	}))
	router.Post("/api/v1alpha1/vms", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	return router
}

func postVMs(body string, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/vms", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	validationRouter().ServeHTTP(rec, req)
	return rec
}

func expectProblemJSON(rec *httptest.ResponseRecorder, status int, title string) server.Error {
	Expect(rec.Code).To(Equal(status))
	Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

	var problem server.Error
	Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
	Expect(problem.Type).To(Equal("about:blank"))
	Expect(problem.Title).To(Equal(title))
	Expect(problem.Status).NotTo(BeNil())
	Expect(*problem.Status).To(Equal(status))
	Expect(problem.Detail).NotTo(BeNil())
	Expect(*problem.Detail).NotTo(BeEmpty())
	return problem
}

var _ = Describe("OpenAPI validation ErrorHandler", func() {
	It("returns problem+json for an empty body", func() {
		rec := postVMs("", "application/json")
		problem := expectProblemJSON(rec, http.StatusBadRequest, "Validation Error")
		Expect(*problem.Detail).To(ContainSubstring("required"))
	})

	It("returns problem+json for malformed JSON", func() {
		rec := postVMs(`{"spec":`, "application/json")
		problem := expectProblemJSON(rec, http.StatusBadRequest, "Validation Error")
		Expect(*problem.Detail).To(ContainSubstring("decode"))
	})

	It("returns problem+json for an unexpected Content-Type", func() {
		rec := postVMs(`{"spec":{}}`, "text/plain")
		problem := expectProblemJSON(rec, http.StatusBadRequest, "Validation Error")
		Expect(*problem.Detail).To(ContainSubstring("Content-Type"))
	})

	It("returns problem+json for a schema violation", func() {
		body := `{
			"spec": {
				"service_type": "vm",
				"metadata": {"name": "test-vm"},
				"vcpu": {"count": 1},
				"memory": {"size": "1Gi"},
				"storage": {"disks": [{"name": "boot", "capacity": "10GB"}]},
				"guest_os": {"type": "fedora-39"}
			}
		}`
		rec := postVMs(body, "application/json")
		problem := expectProblemJSON(rec, http.StatusBadRequest, "Validation Error")
		Expect(*problem.Detail).To(Or(ContainSubstring("memory"), ContainSubstring("pattern"), ContainSubstring("size")))
	})

	It("does not use text/plain", func() {
		rec := httptest.NewRecorder()
		openAPIValidationErrorHandler(rec, "value is required but missing", http.StatusBadRequest)
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		Expect(bytes.Contains(rec.Body.Bytes(), []byte("text/plain"))).To(BeFalse())
	})
})
