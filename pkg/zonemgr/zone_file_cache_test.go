package zonemgr

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"
	"sort"
	"strings"

	k8stypes "k8s.io/apimachinery/pkg/types"
	v1 "kubevirt.io/api/core/v1"
)

var _ = Describe("cached zone file content maintenance", func() {

	const (
		domain       = "domain.com"
		nameServerIP = "185.251.75.10"
	)

	var zoneFileCache *ZoneFileCache

	Describe("cached zone file initialization", func() {
		const (
			headerDefault   = "$ORIGIN vm. \n$TTL 3600 \n@ IN SOA ns.vm. email.vm. (0 3600 3600 1209600 3600)\n"
			headerCustomFmt = "$ORIGIN vm.%s. \n$TTL 3600 \n@ IN SOA ns.vm.%s. email.vm.%s. (0 3600 3600 1209600 3600)\nIN NS ns.vm.%s.\nIN A %s\n"
		)

		var (
			headerCustom = fmt.Sprintf(headerCustomFmt, domain, domain, domain, domain, nameServerIP)
		)

		DescribeTable("generate zone file header", func(nameServerIP, domain, expectedHeader string) {
			zoneFileCache = NewZoneFileCache(nameServerIP, domain)
			zoneFileCache.init()
			Expect(zoneFileCache.header).To(Equal(expectedHeader))
		},
			Entry("header should contain default values", "", "", headerDefault),
			Entry("header should contain custom values", nameServerIP, domain, headerCustom),
		)
	})

	Describe("cached zone file records update", func() {
		const (
			vmi1Name      = "vmi1"
			vmi2Name      = "vmi2"
			vmi1Namespace = "ns1"
			vmi2Namespace = "ns2"
			nic1Name      = "nic1"
			nic1IP        = "1.2.3.4"
			nic2Name      = "nic2"
			nic2IP        = "5.6.7.8"
			nic3Name      = "nic3"
			nic3IP        = "9.10.11.12"
			nic4Name      = "nic4"
			nic4IP        = "13.14.15.16"
			IPv6          = "fe80::74c8:f2ff:fe5f:ff2b"

			aRecordFmt = "%s.%s.%s IN A %s\n"
		)

		var (
			aRecord_nic1_vm1_ns1 = fmt.Sprintf(aRecordFmt, nic1Name, vmi1Name, vmi1Namespace, nic1IP)
			aRecord_nic2_vm1_ns1 = fmt.Sprintf(aRecordFmt, nic2Name, vmi1Name, vmi1Namespace, nic2IP)
			aRecord_nic4_vm1_ns1 = fmt.Sprintf(aRecordFmt, nic4Name, vmi1Name, vmi1Namespace, nic4IP)

			aRecord_nic1_vm2_ns1 = fmt.Sprintf(aRecordFmt, nic1Name, vmi2Name, vmi1Namespace, nic1IP)
			aRecord_nic2_vm2_ns1 = fmt.Sprintf(aRecordFmt, nic2Name, vmi2Name, vmi1Namespace, nic2IP)
			aRecord_nic3_vm2_ns1 = fmt.Sprintf(aRecordFmt, nic3Name, vmi2Name, vmi1Namespace, nic3IP)
			aRecord_nic4_vm2_ns1 = fmt.Sprintf(aRecordFmt, nic4Name, vmi2Name, vmi1Namespace, nic4IP)

			aRecord_nic3_vm1_ns2 = fmt.Sprintf(aRecordFmt, nic3Name, vmi1Name, vmi2Namespace, nic3IP)
			aRecord_nic4_vm1_ns2 = fmt.Sprintf(aRecordFmt, nic4Name, vmi1Name, vmi2Namespace, nic4IP)
		)

		validateUpdateFunc := func(newVmiName, newVmiNamespace string, newInterfaces []v1.VirtualMachineInstanceNetworkInterface,
			expectedIsUpdated bool, expectedRecords string, expectedSoaSerial int) {
			isUpdated := zoneFileCache.updateVMIRecords(k8stypes.NamespacedName{newVmiNamespace, newVmiName}, newInterfaces)
			Expect(isUpdated).To(Equal(expectedIsUpdated))
			Expect(sortRecords(zoneFileCache.aRecords)).To(Equal(sortRecords(expectedRecords)))
			Expect(zoneFileCache.soaSerial).To(Equal(expectedSoaSerial))
		}

		When("interfaces records list is empty", func() {
			BeforeEach(func() {
				zoneFileCache = NewZoneFileCache(nameServerIP, domain)
				zoneFileCache.init()
			})

			DescribeTable("Updating interfaces records", validateUpdateFunc,
				Entry("when new vmi with interfaces list is added",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}},
					true,
					aRecord_nic1_vm1_ns1,
					1,
				),
				Entry("when non existing vmi is deleted",
					vmi1Name,
					vmi1Namespace,
					nil,
					false,
					"",
					0,
				),
			)
		})

		When("interfaces records list contains single vmi", func() {
			BeforeEach(func() {
				zoneFileCache = NewZoneFileCache(nameServerIP, domain)
				zoneFileCache.init()
				isUpdated := zoneFileCache.updateVMIRecords(k8stypes.NamespacedName{vmi1Namespace, vmi1Name},
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}, {IPs: []string{nic2IP}, Name: nic2Name}})
				Expect(isUpdated).To(BeTrue())
			})

			DescribeTable("Updating interfaces records list", validateUpdateFunc,
				Entry("when new vmi with interfaces list is added",
					vmi2Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic3IP}, Name: nic3Name}, {IPs: []string{nic4IP}, Name: nic4Name}},
					true,
					aRecord_nic1_vm1_ns1+
						aRecord_nic2_vm1_ns1+
						aRecord_nic3_vm2_ns1+
						aRecord_nic4_vm2_ns1,
					2,
				),
				Entry("when existing vmi is deleted",
					vmi1Name,
					vmi1Namespace,
					nil,
					true,
					"",
					2,
				),
				Entry("when existing vmi interfaces list is changed",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}, {IPs: []string{nic4IP}, Name: nic4Name}},
					true,
					aRecord_nic1_vm1_ns1+
						aRecord_nic4_vm1_ns1,
					2,
				),
				Entry("when existing vmi is not changed but its interfaces order is changed",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic2IP}, Name: nic2Name}, {IPs: []string{nic1IP}, Name: nic1Name}},
					false,
					aRecord_nic1_vm1_ns1+
						aRecord_nic2_vm1_ns1,
					1,
				),
			)
		})

		When("interfaces records list contains multiple vmis", func() {
			BeforeEach(func() {
				zoneFileCache = NewZoneFileCache(nameServerIP, domain)
				zoneFileCache.init()
				isUpdated := zoneFileCache.updateVMIRecords(k8stypes.NamespacedName{vmi1Namespace, vmi1Name},
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}, {IPs: []string{nic2IP}, Name: nic2Name}})
				Expect(isUpdated).To(BeTrue())
				isUpdated = zoneFileCache.updateVMIRecords(k8stypes.NamespacedName{vmi1Namespace, vmi2Name},
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}, {IPs: []string{nic2IP}, Name: nic2Name}})
				Expect(isUpdated).To(BeTrue())
			})

			DescribeTable("update interfaces records list", validateUpdateFunc,
				Entry("when new vmi with interfaces list is added",
					vmi1Name,
					vmi2Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic3IP}, Name: nic3Name}, {IPs: []string{nic4IP}, Name: nic4Name}},
					true,
					aRecord_nic1_vm1_ns1+
						aRecord_nic2_vm1_ns1+
						aRecord_nic1_vm2_ns1+
						aRecord_nic2_vm2_ns1+
						aRecord_nic3_vm1_ns2+
						aRecord_nic4_vm1_ns2,
					3,
				),
				Entry("when existing vmi is deleted",
					vmi1Name,
					vmi1Namespace,
					nil,
					true,
					aRecord_nic1_vm2_ns1+
						aRecord_nic2_vm2_ns1,
					3,
				),
				Entry("when existing vmi interfaces list is changed",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP}, Name: nic1Name}, {IPs: []string{nic4IP}, Name: nic4Name}},
					true,
					aRecord_nic1_vm1_ns1+
						aRecord_nic4_vm1_ns1+
						aRecord_nic1_vm2_ns1+
						aRecord_nic2_vm2_ns1,
					3,
				),
				Entry("when existing vmi is not changed but its interfaces order is changed",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic2IP}, Name: nic2Name}, {IPs: []string{nic1IP}, Name: nic1Name}},
					false,
					aRecord_nic2_vm1_ns1+
						aRecord_nic1_vm1_ns1+
						aRecord_nic1_vm2_ns1+
						aRecord_nic2_vm2_ns1,
					2,
				),
			)
		})

		When("interfaces records list contains vmi with multiple IPs", func() {
			BeforeEach(func() {
				zoneFileCache = NewZoneFileCache(nameServerIP, domain)
				zoneFileCache.init()
			})

			DescribeTable("Updating interfaces records list", validateUpdateFunc,
				Entry("vmi interfaces contain IPv4 and IPv6",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{nic1IP, IPv6}, Name: nic1Name}, {IPs: []string{nic2IP, IPv6}, Name: nic2Name}},
					true,
					aRecord_nic1_vm1_ns1+
						aRecord_nic2_vm1_ns1,
					1,
				),
				Entry("vmi interfaces contain IPv6 only",
					vmi1Name,
					vmi1Namespace,
					[]v1.VirtualMachineInstanceNetworkInterface{{IPs: []string{IPv6}, Name: nic1Name}, {IPs: []string{IPv6}, Name: nic2Name}},
					false,
					"",
					0,
				),
			)
		})
	})
})

func sortRecords(recordsStr string) (sortedRecordsStr string) {
	strArr := strings.Split(recordsStr, "\n")
	sort.Strings(strArr)
	return strings.Join(strArr, "\n")
}
