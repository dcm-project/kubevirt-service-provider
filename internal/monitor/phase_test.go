package monitor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/dcm-project/kubevirt-service-provider/internal/constants"
)

func TestMonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Monitor Suite")
}

var _ = Describe("Phase", func() {
	Describe("ExtractVMInfo", func() {
		It("should return error for nil VMI", func() {
			info, err := ExtractVMInfo(nil)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("VMI object is nil"))
			Expect(info).To(Equal(VMInfo{}))
		})

		It("should extract info from a valid VMI with labels", func() {
			vmi := &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					Labels: map[string]string{
						constants.DCMLabelInstanceID: "vm-123",
					},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Phase: kubevirtv1.Running,
				},
			}

			info, err := ExtractVMInfo(vmi)

			Expect(err).NotTo(HaveOccurred())
			Expect(info.VMID).To(Equal("vm-123"))
			Expect(info.VMName).To(Equal("test-vm"))
			Expect(info.Namespace).To(Equal("default"))
			Expect(info.Phase).To(Equal(VMPhaseRunning))
		})

		It("should return empty VMID when DCM label is missing", func() {
			vmi := &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vm",
					Namespace: "default",
					Labels:    map[string]string{},
				},
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					Phase: kubevirtv1.Pending,
				},
			}

			info, err := ExtractVMInfo(vmi)

			Expect(err).NotTo(HaveOccurred())
			Expect(info.VMID).To(BeEmpty())
			Expect(info.VMName).To(Equal("test-vm"))
			Expect(info.Phase).To(Equal(VMPhasePending))
		})
	})

	Describe("mapVMIPhase", func() {
		DescribeTable("should map KubeVirt phases correctly",
			func(input kubevirtv1.VirtualMachineInstancePhase, expected VMPhase) {
				Expect(mapVMIPhase(input)).To(Equal(expected))
			},
			Entry("Pending", kubevirtv1.Pending, VMPhasePending),
			Entry("Scheduling", kubevirtv1.Scheduling, VMPhaseScheduling),
			Entry("Scheduled", kubevirtv1.Scheduled, VMPhaseScheduled),
			Entry("Running", kubevirtv1.Running, VMPhaseRunning),
			Entry("Succeeded", kubevirtv1.Succeeded, VMPhaseSucceeded),
			Entry("Failed", kubevirtv1.Failed, VMPhaseFailed),
			Entry("Unknown", kubevirtv1.Unknown, VMPhaseUnknown),
		)

		It("should default unknown phases to Unknown", func() {
			result := mapVMIPhase(kubevirtv1.VirtualMachineInstancePhase("SomeNewPhase"))
			Expect(result).To(Equal(VMPhaseUnknown))
		})
	})
})
