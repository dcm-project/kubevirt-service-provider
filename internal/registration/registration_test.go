package registration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	spmv1alpha1 "github.com/dcm-project/service-provider-manager/api/v1alpha1/provider"

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
			HTTPTimeout:   30 * time.Second,
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
			Expect(registrar.initialBackoff).To(Equal(1 * time.Second))
			Expect(registrar.maxBackoff).To(Equal(60 * time.Second))
		})

		It("should accept custom backoff options", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			svcMgrCfg = &config.ServiceProviderManagerConfig{
				Endpoint: testServer.URL,
			}

			registrar, err := NewRegistrar(providerCfg, svcMgrCfg,
				SetInitialBackoff(100*time.Millisecond),
				SetMaxBackoff(5*time.Second),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(registrar.initialBackoff).To(Equal(100 * time.Millisecond))
			Expect(registrar.maxBackoff).To(Equal(5 * time.Second))
		})
	})

	Describe("Start", func() {
		Context("when registration succeeds with new provider", func() {
			It("should complete registration in the background", func() {
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
					providerUUID := validUUID
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

				registrar.Start(context.Background())
				Eventually(registrar.Done()).Should(BeClosed())
			})
		})

		Context("when provider already exists and is updated", func() {
			It("should complete registration in the background", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					providerUUID := validUUID
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

				registrar.Start(context.Background())
				Eventually(registrar.Done()).Should(BeClosed())
			})
		})

		Context("when there is a non-retryable error", func() {
			It("should give up on conflict", func() {
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

				registrar.Start(context.Background())
				Eventually(registrar.Done()).Should(BeClosed())
			})

			It("should give up on validation error", func() {
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

				registrar.Start(context.Background())
				Eventually(registrar.Done()).Should(BeClosed())
			})

			It("should give up on invalid UUID", func() {
				providerCfg.ID = "invalid-uuid"

				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Fail("Server should not be called with invalid UUID")
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg)
				Expect(err).NotTo(HaveOccurred())

				registrar.Start(context.Background())
				Eventually(registrar.Done()).Should(BeClosed())
			})
		})

		Context("when there is a retryable error", func() {
			It("should retry and eventually succeed", func() {
				var attempts int32
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempt := atomic.AddInt32(&attempts, 1)
					if attempt < 3 {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					providerUUID := validUUID
					response := spmv1alpha1.Provider{
						Id:   &providerUUID,
						Name: "test-provider",
					}
					json.NewEncoder(w).Encode(response)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg,
					SetInitialBackoff(10*time.Millisecond),
					SetMaxBackoff(50*time.Millisecond),
				)
				Expect(err).NotTo(HaveOccurred())

				registrar.Start(context.Background())
				Eventually(registrar.Done(), 5*time.Second).Should(BeClosed())
				Expect(atomic.LoadInt32(&attempts)).To(BeNumerically(">=", int32(3)))
			})
		})

		Context("when context is cancelled", func() {
			It("should stop retrying", func() {
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))

				svcMgrCfg = &config.ServiceProviderManagerConfig{
					Endpoint: testServer.URL,
				}

				registrar, err := NewRegistrar(providerCfg, svcMgrCfg,
					SetInitialBackoff(10*time.Millisecond),
				)
				Expect(err).NotTo(HaveOccurred())

				ctx, cancel := context.WithCancel(context.Background())
				registrar.Start(ctx)

				// Give it time to fail at least once then cancel
				time.Sleep(50 * time.Millisecond)
				cancel()

				Eventually(registrar.Done()).Should(BeClosed())
			})
		})

		Context("when Start is called multiple times", func() {
			It("should only start one registration goroutine", func() {
				var attempts int32
				testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					atomic.AddInt32(&attempts, 1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					providerUUID := validUUID
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

				ctx := context.Background()
				registrar.Start(ctx)
				registrar.Start(ctx)
				registrar.Start(ctx)

				Eventually(registrar.Done()).Should(BeClosed())
				Expect(atomic.LoadInt32(&attempts)).To(Equal(int32(1)))
			})
		})
	})
})
