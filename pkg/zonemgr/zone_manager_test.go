package zonemgr

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"os"

	k8stypes "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Zone Manager functionality", func() {

	const (
		customDomain = "domain.com"
		customNSIP   = "1.2.3.4"
	)

	var zoneMgr *ZoneManager

	BeforeEach(func() {
		os.Setenv("DOMAIN", customDomain)
		os.Setenv("NAME_SERVER_IP", customNSIP)
		zoneMgr = NewZoneManager()
	})

	AfterEach(func() {
	})

	Context("Initialization", func() {
		It("should validate input data", func() {
			Expect(zoneMgr.UpdateZone(k8stypes.NamespacedName{"", "vm1"}, nil)).NotTo(Succeed())
			Expect(zoneMgr.UpdateZone(k8stypes.NamespacedName{"ns1", ""}, nil)).NotTo(Succeed())
		})

		It("should set custom data", func() {
			Expect(zoneMgr.UpdateZone(k8stypes.NamespacedName{"ns1", "vm1"}, nil)).To(Succeed())
			Expect(zoneMgr.zoneFileCache.domain).To(Equal("vm." + customDomain))
			Expect(zoneMgr.zoneFileCache.nameServerIP).To(Equal(customNSIP))
		})

		It("should create zone file with correct name", func() {
			Expect(zoneMgr.zoneFile.zoneFileFullName).To(Equal("/zones/db.vm." + customDomain))
		})

		When("zone file already exist", func() {
			It("should update cached SOA serial value", func() {
				//todo
			})
		})
	})
})
