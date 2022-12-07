package zonemgr

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/net"

	v1 "kubevirt.io/api/core/v1"
)

const (
	refresh = "3600"    // 1 hour (seconds) - how long a nameserver should wait prior to checking for a Serial Number increase within the primary zone file
	retry   = "3600"    // 1 hour (seconds) - how long a nameserver should wait prior to retrying to update a zone after a failed attempt.
	expire  = "1209600" // 2 weeks (seconds) - how long a nameserver should wait prior to considering data from a secondary zone invalid and stop answering queries for that zone
	ttl     = "3600"    // 1 hour (seconds) - the duration that the record may be cached by any resolver

	domainDefault     = "vm"
	nameServerDefault = "ns"
	adminEmailDefault = "email"
)

type ZoneFileCache struct {
	soaSerial      int
	adminEmail     string
	nameServerName string
	nameServerIP   string
	domain         string

	headerPref string
	headerSuf  string

	header   string
	aRecords string
	content  string

	vmiRecordsMap map[string][]string
}

func NewZoneFileCache(nameServerIP string, domain string) *ZoneFileCache {
	zoneFileCache := &ZoneFileCache{
		nameServerIP: nameServerIP,
		domain:       domain,
	}
	zoneFileCache.prepare()
	return zoneFileCache
}

func (zoneFileCache *ZoneFileCache) prepare() {
	zoneFileCache.initCustomFields()
	zoneFileCache.generateHeaderPrefix()
	zoneFileCache.generateHeaderSuffix()
	zoneFileCache.soaSerial = 0
	zoneFileCache.header = zoneFileCache.generateHeader()
	zoneFileCache.vmiRecordsMap = make(map[string][]string)
}

func (zoneFileCache *ZoneFileCache) updateIfAlreadyExist(content []byte) {
	if prevSoaSerial := fetchSoaSerial(content); prevSoaSerial > 0 {
		zoneFileCache.soaSerial = prevSoaSerial + 1
		zoneFileCache.header = zoneFileCache.generateHeader()
	}
}

func fetchSoaSerial(content []byte) int {
	contentStr := string(content)
	var ind1, ind2 int
	if ind1 = strings.Index(contentStr, "("); ind1 == 0 {
		return 0
	}
	if ind2 = strings.Index(contentStr[ind1:], " "); ind2 == 0 {
		return 0
	}
	soaSerial := strings.TrimSpace(contentStr[ind1+1 : ind1+ind2])
	if soaSerialInt, err := strconv.Atoi(soaSerial); err != nil {
		return 0
	} else {
		return soaSerialInt
	}
}

func (zoneFileCache *ZoneFileCache) initCustomFields() {
	if zoneFileCache.domain == "" {
		zoneFileCache.domain = domainDefault
	} else {
		zoneFileCache.domain = fmt.Sprintf("%s.%s", domainDefault, zoneFileCache.domain)
	}
	zoneFileCache.nameServerName = fmt.Sprintf("%s.%s", nameServerDefault, zoneFileCache.domain)
	zoneFileCache.adminEmail = fmt.Sprintf("%s.%s", adminEmailDefault, zoneFileCache.domain)
}

func (zoneFileCache *ZoneFileCache) generateHeaderPrefix() {
	zoneFileCache.headerPref = fmt.Sprintf("$ORIGIN %s. \n$TTL %s \n@ IN SOA %s. %s. (", zoneFileCache.domain, ttl,
		zoneFileCache.nameServerName, zoneFileCache.adminEmail)
}

func (zoneFileCache *ZoneFileCache) generateHeaderSuffix() {
	zoneFileCache.headerSuf = fmt.Sprintf(" %s %s %s %s)\n", refresh, retry, expire, ttl)

	if zoneFileCache.nameServerIP != "" {
		zoneFileCache.headerSuf += fmt.Sprintf("IN NS %s.\n", zoneFileCache.nameServerName)
		zoneFileCache.headerSuf += fmt.Sprintf("IN A %s\n", zoneFileCache.nameServerIP)
	}
}

func (zoneFileCache *ZoneFileCache) generateHeader() string {
	return zoneFileCache.headerPref + strconv.Itoa(zoneFileCache.soaSerial) + zoneFileCache.headerSuf
}

func (zoneFileCache *ZoneFileCache) updateVMIRecords(namespacedName k8stypes.NamespacedName, interfaces []v1.VirtualMachineInstanceNetworkInterface) bool {
	key := fmt.Sprintf("%s_%s", namespacedName.Name, namespacedName.Namespace)
	isUpdated := false

	if interfaces == nil {
		if zoneFileCache.vmiRecordsMap[key] != nil {
			delete(zoneFileCache.vmiRecordsMap, key)
			isUpdated = true
		}
	} else {
		newRecords := buildARecordsArr(namespacedName.Name, namespacedName.Namespace, interfaces)
		isUpdated = !reflect.DeepEqual(newRecords, zoneFileCache.vmiRecordsMap[key])
		if isUpdated {
			zoneFileCache.vmiRecordsMap[key] = newRecords
		}
	}

	if isUpdated {
		zoneFileCache.updateContent()
	}
	return isUpdated
}

func buildARecordsArr(name string, namespace string, interfaces []v1.VirtualMachineInstanceNetworkInterface) []string {
	var recordsArr []string
	for _, iface := range interfaces {
		if iface.Name != "" {
			IPs := iface.IPs
			for _, IP := range IPs {
				if net.IsIPv4String(IP) {
					recordsArr = append(recordsArr, generateARecord(name, namespace, iface.Name, IP))
					break
				}
			}
		}
	}
	sort.Strings(recordsArr)
	return recordsArr
}

func generateARecord(name string, namespace string, ifaceName string, ifaceIP string) string {
	fqdn := fmt.Sprintf("%s.%s.%s", ifaceName, name, namespace)
	return fmt.Sprintf("%s IN A %s\n", fqdn, ifaceIP)
}

func (zoneFileCache ZoneFileCache) generateARecords() string {
	aRecords := ""
	for _, recordsArr := range zoneFileCache.vmiRecordsMap {
		for _, aRecord := range recordsArr {
			aRecords += aRecord
		}
	}
	return aRecords
}

func (zoneFileCache *ZoneFileCache) updateContent() {
	zoneFileCache.soaSerial++
	zoneFileCache.header = zoneFileCache.generateHeader()
	zoneFileCache.aRecords = zoneFileCache.generateARecords()

	zoneFileCache.content = zoneFileCache.header + zoneFileCache.aRecords
}
