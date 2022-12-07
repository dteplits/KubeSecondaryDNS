package zonemgr

import (
	"errors"
	"os"

	k8stypes "k8s.io/apimachinery/pkg/types"

	v1 "kubevirt.io/api/core/v1"
)

const (
	zoneFileName       = "/zones/db."
	envVarDomain       = "DOMAIN"
	envVarNameServerIP = "NAME_SERVER_IP"
)

type SecIfaceData struct {
	interfaceName string
	interfaceIP   string
	namespaceName string
	vmName        string
}

type ZoneManager struct {
	zoneFileCache *ZoneFileCache
	zoneFile      *ZoneFile
}

func NewZoneManager() *ZoneManager {
	zoneMgr := &ZoneManager{}
	zoneMgr.prepare()
	return zoneMgr
}

func (zoneMgr *ZoneManager) prepare() {
	domain := os.Getenv(envVarDomain)
	nameServerIP := os.Getenv(envVarNameServerIP)

	zoneMgr.zoneFileCache = NewZoneFileCache(nameServerIP, domain)

	zoneMgr.zoneFile = NewZoneFile(zoneFileName + zoneMgr.zoneFileCache.domain)

	if content, _ := zoneMgr.zoneFile.readFile(); content != nil && len(content) > 0 {
		zoneMgr.zoneFileCache.updateIfAlreadyExist(content)
	}
}

func (zoneMgr *ZoneManager) UpdateZone(namespacedName k8stypes.NamespacedName, interfaces []v1.VirtualMachineInstanceNetworkInterface) error {
	if namespacedName.Name == "" {
		return errors.New("VM name in empty")
	}
	if namespacedName.Namespace == "" {
		return errors.New("VM namespace is empty")
	}

	if isUpdated := zoneMgr.zoneFileCache.updateVMIRecords(namespacedName, interfaces); isUpdated {
		return zoneMgr.zoneFile.writeFile(zoneMgr.zoneFileCache.content)
	}

	return nil
}
