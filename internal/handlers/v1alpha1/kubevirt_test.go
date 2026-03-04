package v1alpha1

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openapi_types "github.com/oapi-codegen/runtime/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	types "github.com/dcm-project/kubevirt-service-provider/api/v1alpha1"
	"github.com/dcm-project/kubevirt-service-provider/internal/api/server"
	"github.com/dcm-project/kubevirt-service-provider/internal/constants"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newTestVM(vmID string) *kubevirtv1.VirtualMachine {
	return &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dcm-test-vm",
			Namespace: "default",
			Labels: map[string]string{
				constants.DCMLabelInstanceID: vmID,
				constants.DCMLabelManagedBy: constants.DCMManagedByValue,
			},
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.DCMLabelInstanceID: vmID,
						constants.DCMLabelManagedBy: constants.DCMManagedByValue,
					},
				},
			},
		},
	}
}

func newTestVMSpec() *types.VMSpec {
	return &types.VMSpec{
		ServiceType: types.Vm,
		Metadata: types.ServiceMetadata{
			Name: "test-vm",
		},
		GuestOs: types.GuestOS{
			Type: "ubuntu",
		},
		Vcpu: types.Vcpu{
			Count: 2,
		},
		Memory: types.Memory{
			Size: "2Gi",
		},
		Storage: types.Storage{
			Disks: []types.Disk{
				{Name: "boot", Capacity: "10Gi"},
			},
		},
	}
}

func newNotFoundError() error {
	return apierrors.NewNotFound(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"}, "test-vm")
}

func newConflictError() error {
	return apierrors.NewConflict(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachines"}, "test-vm", fmt.Errorf("already exists"))
}

var _ = Describe("KubevirtHandler", func() {
	var (
		client   *mockVMClient
		mapper   *mockVMMapper
		h        *KubevirtHandler
		ctx      context.Context
		testID   string
		testUUID openapi_types.UUID
	)

	BeforeEach(func() {
		client = &mockVMClient{}
		mapper = &mockVMMapper{}
		h = NewKubevirtHandler(client, mapper)
		ctx = context.Background()
		testID = "00000000-0000-0000-0000-000000000001"
		testUUID = openapi_types.UUID(uuid.MustParse(testID))
	})

	Describe("GetHealth", func() {
		It("should return 200 with status ok", func() {
			resp, err := h.GetHealth(ctx, server.GetHealthRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			healthResp, ok := resp.(server.GetHealth200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*healthResp.Status).To(Equal("ok"))
			Expect(*healthResp.Path).To(Equal("/api/v1alpha1/health"))
		})
	})

	Describe("ListVMs", func() {
		It("should return VMs successfully", func() {
			vm := newTestVM(testID)
			client.listFn = func(_ context.Context, opts metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
				Expect(opts.LabelSelector).To(ContainSubstring(constants.DCMLabelManagedBy))
				return []kubevirtv1.VirtualMachine{*vm}, nil
			}
			mapper.vmToVMSpecFn = func(_ *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
				return newTestVMSpec(), nil
			}

			resp, err := h.ListVMs(ctx, server.ListVMsRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			listResp, ok := resp.(server.ListVMs200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*listResp.Vms).To(HaveLen(1))
		})

		It("should return an empty list when no VMs exist", func() {
			client.listFn = func(_ context.Context, _ metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
				return []kubevirtv1.VirtualMachine{}, nil
			}

			resp, err := h.ListVMs(ctx, server.ListVMsRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			listResp, ok := resp.(server.ListVMs200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*listResp.Vms).To(HaveLen(0))
		})

		It("should return an error response when client fails", func() {
			client.listFn = func(_ context.Context, _ metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
				return nil, fmt.Errorf("connection refused")
			}

			resp, err := h.ListVMs(ctx, server.ListVMsRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(*server.ListVMsdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("should skip VMs that fail conversion with a warning", func() {
			vm1 := newTestVM(testID)
			vm2 := newTestVM("00000000-0000-0000-0000-000000000002")
			client.listFn = func(_ context.Context, _ metav1.ListOptions) ([]kubevirtv1.VirtualMachine, error) {
				return []kubevirtv1.VirtualMachine{*vm1, *vm2}, nil
			}
			callCount := 0
			mapper.vmToVMSpecFn = func(_ *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
				callCount++
				if callCount == 1 {
					return nil, fmt.Errorf("conversion error")
				}
				return newTestVMSpec(), nil
			}

			resp, err := h.ListVMs(ctx, server.ListVMsRequestObject{})

			Expect(err).NotTo(HaveOccurred())
			listResp, ok := resp.(server.ListVMs200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*listResp.Vms).To(HaveLen(1))
		})
	})

	Describe("CreateVM", func() {
		var request server.CreateVMRequestObject

		BeforeEach(func() {
			body := server.CreateVMJSONRequestBody{
				VmSpec: server.VMSpec{
					ServiceType: server.Vm,
					Metadata:    server.ServiceMetadata{Name: "test-vm"},
					GuestOs:     server.GuestOS{Type: "ubuntu"},
					Vcpu:        server.Vcpu{Count: 2},
					Memory:      server.Memory{Size: "2Gi"},
					Storage:     server.Storage{Disks: []server.Disk{{Name: "boot", Capacity: "10Gi"}}},
				},
			}
			request = server.CreateVMRequestObject{
				Params: server.CreateVMParams{Id: &testUUID},
				Body:   &body,
			}
		})

		It("should create a VM successfully and return 201", func() {
			mapper.vmSpecToVMFn = func(_ *types.VMSpec, _ string) (*kubevirtv1.VirtualMachine, error) {
				return newTestVM(testID), nil
			}
			client.createFn = func(_ context.Context, vm *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
				return vm, nil
			}
			mapper.vmToVMSpecFn = func(_ *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
				return newTestVMSpec(), nil
			}

			resp, err := h.CreateVM(ctx, request)

			Expect(err).NotTo(HaveOccurred())
			createResp, ok := resp.(server.CreateVM201JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*createResp.Path).To(ContainSubstring(testID))
		})

		It("should return error when client create fails", func() {
			mapper.vmSpecToVMFn = func(_ *types.VMSpec, _ string) (*kubevirtv1.VirtualMachine, error) {
				return newTestVM(testID), nil
			}
			client.createFn = func(_ context.Context, _ *kubevirtv1.VirtualMachine) (*kubevirtv1.VirtualMachine, error) {
				return nil, newConflictError()
			}

			resp, err := h.CreateVM(ctx, request)

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(*server.CreateVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusConflict))
		})

		It("should return validation error when mapper conversion fails", func() {
			mapper.vmSpecToVMFn = func(_ *types.VMSpec, _ string) (*kubevirtv1.VirtualMachine, error) {
				return nil, fmt.Errorf("invalid memory format")
			}

			resp, err := h.CreateVM(ctx, request)

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(*server.CreateVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("DeleteVM", func() {
		It("should delete a VM successfully and return 204", func() {
			client.deleteFn = func(_ context.Context, _ string) error {
				return nil
			}

			resp, err := h.DeleteVM(ctx, server.DeleteVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			_, ok := resp.(server.DeleteVM204Response)
			Expect(ok).To(BeTrue())
		})

		It("should return 404 when VM is not found", func() {
			client.deleteFn = func(_ context.Context, _ string) error {
				return newNotFoundError()
			}

			resp, err := h.DeleteVM(ctx, server.DeleteVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			notFoundResp, ok := resp.(server.DeleteVM404ApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*notFoundResp.Status).To(Equal(404))
		})

		It("should return error when delete fails", func() {
			client.deleteFn = func(_ context.Context, _ string) error {
				return fmt.Errorf("connection refused")
			}

			resp, err := h.DeleteVM(ctx, server.DeleteVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(server.DeleteVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("GetVM", func() {
		It("should return a VM successfully", func() {
			client.getFn = func(_ context.Context, _ string) (*kubevirtv1.VirtualMachine, error) {
				return newTestVM(testID), nil
			}
			mapper.vmToVMSpecFn = func(_ *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
				return newTestVMSpec(), nil
			}

			resp, err := h.GetVM(ctx, server.GetVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			vmResp, ok := resp.(server.GetVM200JSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*vmResp.Path).To(ContainSubstring(testID))
		})

		It("should return 404 when VM is not found", func() {
			client.getFn = func(_ context.Context, _ string) (*kubevirtv1.VirtualMachine, error) {
				return nil, newNotFoundError()
			}

			resp, err := h.GetVM(ctx, server.GetVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			notFoundResp, ok := resp.(server.GetVM404ApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(*notFoundResp.Status).To(Equal(404))
		})

		It("should return error when client fails with non-404", func() {
			client.getFn = func(_ context.Context, _ string) (*kubevirtv1.VirtualMachine, error) {
				return nil, fmt.Errorf("connection refused")
			}

			resp, err := h.GetVM(ctx, server.GetVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(server.GetVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("should return error when mapper conversion fails", func() {
			client.getFn = func(_ context.Context, _ string) (*kubevirtv1.VirtualMachine, error) {
				return newTestVM(testID), nil
			}
			mapper.vmToVMSpecFn = func(_ *kubevirtv1.VirtualMachine) (*types.VMSpec, error) {
				return nil, fmt.Errorf("conversion error")
			}

			resp, err := h.GetVM(ctx, server.GetVMRequestObject{VmId: testUUID})

			Expect(err).NotTo(HaveOccurred())
			errResp, ok := resp.(server.GetVMdefaultApplicationProblemPlusJSONResponse)
			Expect(ok).To(BeTrue())
			Expect(errResp.StatusCode).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("extractVMIDFromVM", func() {
		It("should extract ID from main labels", func() {
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.DCMLabelInstanceID: testID,
					},
				},
			}

			vmID := h.extractVMIDFromVM(vm)
			Expect(vmID).To(Equal(testID))
		})

		It("should extract ID from template labels when main labels missing", func() {
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								constants.DCMLabelInstanceID: testID,
							},
						},
					},
				},
			}

			vmID := h.extractVMIDFromVM(vm)
			Expect(vmID).To(Equal(testID))
		})

		It("should return empty string when no ID found", func() {
			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			}

			vmID := h.extractVMIDFromVM(vm)
			Expect(vmID).To(BeEmpty())
		})
	})
})
