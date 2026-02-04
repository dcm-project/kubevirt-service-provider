package registration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spmv1alpha1 "github.com/dcm-project/service-provider-manager/api/v1alpha1"

	"github.com/dcm-project/kubevirt-service-provider/internal/config"
)

var _ = Describe("Registrar", func() {
	var (
		providerCfg *config.ProviderConfig
		svcMgrCfg   *config.ServiceProviderManagerConfig
		testServer  *httptest.Server
		validUUID   string
	)

	BeforeEach(func() {
		validUUID = uuid.New().String()
		providerCfg = &config.ProviderConfig{
			ID:            validUUID,
			Name:          "test-provider",
			Endpoint:      "http://localhost:8081/api/v1alpha1",
			ServiceType:   "vm",
			SchemaVersion: "v1alpha1",
		}
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Describe("NewRegistrar", func() {
		It("should create a registrar with valid configuration", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			svcMgrCfg = &config.ServiceProviderManagerConfig{
				Endpoint: testServer.URL,
			}

			registrar, err := NewRegistrar(providerCfg, svcMgrCfg)

			Expect(err).NotTo(HaveOccurred())
			Expect(registrar).NotTo(BeNil())
			Expect(registrar.providerCfg).To(Equal(providerCfg))
			Expect(registrar.client).NotTo(BeNil())
		})

	})

	Describe("Register", func() {
		Context("when registration succeeds with new provider", func() {
			It("should return nil and log registration", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal(http.MethodPost))
					Expect(r.URL.Path).To(Equal("/providers"))

					// Verify the request body
					var provider spmv1alpha1.Provider
					err := json.NewDecoder(r.Body).Decode(&provider)
					Expect(err).NotTo(HaveOccurred())
					Expect(provider.Name).To(Equal("test-provider"))
					Expect(provider.Endpoint).To(Equal("http://localhost:8081/api/v1alpha1"))
					Expect(provider.ServiceType).To(Equal("vm"))
					Expect(provider.SchemaVersion).To(Equal("v1alpha1"))

					// Verify query parameter
					Expect(r.URL.Query().Get("id")).To(Equal(validUUID))

					// Return 201 Created
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					providerUUID := uuid.MustParse(validUUID)
					response := spmv1alpha1.Provider{
						Id:   &providerUUID,
						Name: "test-provider",
					}
					json.NewEncoder(w).Encode(response)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when provider already exists and is updated", func() {
			It("should return nil and log update", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					providerUUID := uuid.MustParse(validUUID)
					response := spmv1alpha1.Provider{
						Id:   &providerUUID,
						Name: "test-provider",
					}
					json.NewEncoder(w).Encode(response)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when there is a conflict", func() {
			It("should return a conflict error", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusConflict)
					problem := spmv1alpha1.Error{
						Title: "Provider already exists with different configuration",
						Type:  "https://example.com/conflict",
					}
					json.NewEncoder(w).Encode(problem)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("conflict registering provider"))
			})
		})

		Context("when there is a validation error", func() {
			It("should return a validation error", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusBadRequest)
					problem := spmv1alpha1.Error{
						Title: "Invalid provider configuration",
						Type:  "https://example.com/validation-error",
					}
					json.NewEncoder(w).Encode(problem)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation error"))
			})
		})

		Context("when the provider ID is invalid", func() {
			It("should return an error for invalid UUID", func() {
				providerCfg.ID = "invalid-uuid"

				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Fail("Server should not be called with invalid UUID")
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid provider ID"))
			})
		})

		Context("when the server returns an unexpected status", func() {
			It("should return an unexpected response error", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				err = registrar.Register(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected response status: 500"))
			})
		})

		Context("when the HTTP request fails", func() {
			It("should return an error", func() {
				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: "http://localhost:1", // Port that should fail to connect
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()

				err = registrar.Register(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to register provider"))
			})
		})

		Context("when context is cancelled", func() {
			It("should return a context error", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow response
					time.Sleep(1 * time.Second)
					w.WriteHeader(http.StatusOK)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				err = registrar.Register(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to register provider"))
			})
		})
	})
})
