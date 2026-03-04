package monitor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
	"github.com/dcm-project/kubevirt-service-provider/internal/events"
)

var _ = Describe("Service", func() {
	Describe("handleVMEvent", func() {
		var service *Service

		BeforeEach(func() {
			service = &Service{
				ctx:       context.Background(),
				publisher: &events.Publisher{},
				namespace: "default",
			}
		})

		It("should return early for non-unstructured object", func() {
			Expect(func() {
				service.handleVMEvent("not-an-unstructured", "created")
			}).NotTo(Panic())
		})

		It("should return early for unstructured without valid VMI data", func() {
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": "invalid",
				},
			}
			Expect(func() {
				service.handleVMEvent(u, "created")
			}).NotTo(Panic())
		})

		It("should return early for VMI without DCM label", func() {
			vmi := &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vmi",
					Namespace: "default",
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Phase: kubevirtv1.Running,
				},
			}

			data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(vmi)
			Expect(err).NotTo(HaveOccurred())

			u := &unstructured.Unstructured{Object: data}
			Expect(func() {
				service.handleVMEvent(u, "created")
			}).NotTo(Panic())
		})

		It("should process valid VMI with DCM label without panic", func() {
			vmi := &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vmi",
					Namespace: "default",
					Labels: map[string]string{
						constants.DCMLabelInstanceID: "vm-123",
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Phase: kubevirtv1.Running,
				},
			}

			data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(vmi)
			Expect(err).NotTo(HaveOccurred())

			u := &unstructured.Unstructured{Object: data}
			Expect(func() {
				service.handleVMEvent(u, "created")
			}).NotTo(Panic())
		})
	})

	Describe("publishVMEvent", func() {
		It("should not panic when publisher has nil natsConn", func() {
			service := &Service{
				ctx:       context.Background(),
				publisher: &events.Publisher{},
				namespace: "default",
			}

			vmInfo := VMInfo{
				VMID:      "vm-123",
				VMName:    "test-vm",
				Namespace: "default",
				Phase:     VMPhaseRunning,
			}

			Expect(func() {
				service.publishVMEvent(vmInfo)
			}).NotTo(Panic())
		})
	})

	Describe("NewMonitorService", func() {
		It("should create service with correct fields", func() {
			fakeClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			publisher := &events.Publisher{}
			config := MonitorConfig{
				Namespace:    "test-ns",
				ResyncPeriod: 30 * time.Minute,
			}

			svc := NewMonitorService(fakeClient, publisher, config)

			Expect(svc).NotTo(BeNil())
			Expect(svc.namespace).To(Equal("test-ns"))
			Expect(svc.publisher).To(Equal(publisher))
			Expect(svc.resyncPeriod).To(Equal(30 * time.Minute))
			Expect(svc.dynamicClient).To(Equal(fakeClient))
			Expect(svc.informerFactory).NotTo(BeNil())
			Expect(svc.vmiInformer).NotTo(BeNil())
		})
	})
})
