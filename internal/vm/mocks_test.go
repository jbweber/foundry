package vm

import (
	"fmt"
	"sync"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/internal/config"
)

// mockLibvirtClient is a mock implementation of the libvirtClient interface for testing.
type mockLibvirtClient struct {
	mu sync.Mutex

	// Configurable behavior
	domainLookupByNameFunc func(name string) (libvirt.Domain, error)
	domainDefineXMLFunc    func(xml string) (libvirt.Domain, error)
	domainSetAutostartFunc func(dom libvirt.Domain, autostart int32) error
	domainCreateFunc       func(dom libvirt.Domain) error
	domainDestroyFunc      func(dom libvirt.Domain) error
	domainUndefineFunc     func(dom libvirt.Domain) error

	// Call tracking
	domainLookupByNameCalls []string
	domainDefineXMLCalls    []string
	domainSetAutostartCalls []libvirt.Domain
	domainCreateCalls       []libvirt.Domain
	domainDestroyCalls      []libvirt.Domain
	domainUndefineCalls     []libvirt.Domain
}

// newMockLibvirtClient creates a new mock libvirt client with default behavior.
func newMockLibvirtClient() *mockLibvirtClient {
	m := &mockLibvirtClient{}

	// Default: VM does not exist on first call, but exists after define
	// This simulates the real behavior where lookup fails initially, then succeeds after define
	m.domainLookupByNameFunc = func(name string) (libvirt.Domain, error) {
		// If domain was defined (we have define calls), return the domain
		if len(m.domainDefineXMLCalls) > 0 {
			return libvirt.Domain{Name: name}, nil
		}
		// Otherwise, domain not found
		return libvirt.Domain{}, fmt.Errorf("domain not found: %s", name)
	}

	// Default: define succeeds
	m.domainDefineXMLFunc = func(xml string) (libvirt.Domain, error) {
		return libvirt.Domain{Name: "test-vm"}, nil
	}

	// Default: set autostart succeeds
	m.domainSetAutostartFunc = func(dom libvirt.Domain, autostart int32) error {
		return nil
	}

	// Default: create succeeds
	m.domainCreateFunc = func(dom libvirt.Domain) error {
		return nil
	}

	// Default: destroy succeeds
	m.domainDestroyFunc = func(dom libvirt.Domain) error {
		return nil
	}

	// Default: undefine succeeds
	m.domainUndefineFunc = func(dom libvirt.Domain) error {
		return nil
	}

	return m
}

func (m *mockLibvirtClient) DomainLookupByName(name string) (libvirt.Domain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainLookupByNameCalls = append(m.domainLookupByNameCalls, name)
	return m.domainLookupByNameFunc(name)
}

func (m *mockLibvirtClient) DomainDefineXML(xml string) (libvirt.Domain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainDefineXMLCalls = append(m.domainDefineXMLCalls, xml)
	return m.domainDefineXMLFunc(xml)
}

func (m *mockLibvirtClient) DomainSetAutostart(dom libvirt.Domain, autostart int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainSetAutostartCalls = append(m.domainSetAutostartCalls, dom)
	return m.domainSetAutostartFunc(dom, autostart)
}

func (m *mockLibvirtClient) DomainCreate(dom libvirt.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainCreateCalls = append(m.domainCreateCalls, dom)
	return m.domainCreateFunc(dom)
}

func (m *mockLibvirtClient) DomainDestroy(dom libvirt.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainDestroyCalls = append(m.domainDestroyCalls, dom)
	return m.domainDestroyFunc(dom)
}

func (m *mockLibvirtClient) DomainUndefine(dom libvirt.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainUndefineCalls = append(m.domainUndefineCalls, dom)
	return m.domainUndefineFunc(dom)
}

// mockStorageManager is a mock implementation of the storageManager interface for testing.
type mockStorageManager struct {
	mu sync.Mutex

	// Configurable behavior
	checkDiskSpaceFunc    func(cfg *config.VMConfig) error
	vmDirectoryExistsFunc func(vmName string) (bool, error)
	getVMDirectoryFunc    func(vmName string) string
	createVMDirectoryFunc func(vmName string) error
	createBootDiskFunc    func(cfg *config.VMConfig) error
	createDataDiskFunc    func(vmName string, disk config.DataDiskConfig) error
	writeCloudInitISOFunc func(cfg *config.VMConfig, isoData []byte) error
	deleteVMFunc          func(vmName string) error

	// Call tracking
	checkDiskSpaceCalls    []*config.VMConfig
	vmDirectoryExistsCalls []string
	getVMDirectoryCalls    []string
	createVMDirectoryCalls []string
	createBootDiskCalls    []*config.VMConfig
	createDataDiskCalls    []config.DataDiskConfig
	writeCloudInitISOCalls []*config.VMConfig
	deleteVMCalls          []string
}

// newMockStorageManager creates a new mock storage manager with default behavior.
func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{
		// Default: disk space is sufficient
		checkDiskSpaceFunc: func(cfg *config.VMConfig) error {
			return nil
		},
		// Default: VM directory does not exist
		vmDirectoryExistsFunc: func(vmName string) (bool, error) {
			return false, nil
		},
		// Default: return path
		getVMDirectoryFunc: func(vmName string) string {
			return "/var/lib/libvirt/images/" + vmName
		},
		// Default: all operations succeed
		createVMDirectoryFunc: func(vmName string) error {
			return nil
		},
		createBootDiskFunc: func(cfg *config.VMConfig) error {
			return nil
		},
		createDataDiskFunc: func(vmName string, disk config.DataDiskConfig) error {
			return nil
		},
		writeCloudInitISOFunc: func(cfg *config.VMConfig, isoData []byte) error {
			return nil
		},
		deleteVMFunc: func(vmName string) error {
			return nil
		},
	}
}

func (m *mockStorageManager) CheckDiskSpace(cfg *config.VMConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkDiskSpaceCalls = append(m.checkDiskSpaceCalls, cfg)
	return m.checkDiskSpaceFunc(cfg)
}

func (m *mockStorageManager) VMDirectoryExists(vmName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vmDirectoryExistsCalls = append(m.vmDirectoryExistsCalls, vmName)
	return m.vmDirectoryExistsFunc(vmName)
}

func (m *mockStorageManager) GetVMDirectory(vmName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getVMDirectoryCalls = append(m.getVMDirectoryCalls, vmName)
	return m.getVMDirectoryFunc(vmName)
}

func (m *mockStorageManager) CreateVMDirectory(vmName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createVMDirectoryCalls = append(m.createVMDirectoryCalls, vmName)
	return m.createVMDirectoryFunc(vmName)
}

func (m *mockStorageManager) CreateBootDisk(cfg *config.VMConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createBootDiskCalls = append(m.createBootDiskCalls, cfg)
	return m.createBootDiskFunc(cfg)
}

func (m *mockStorageManager) CreateDataDisk(vmName string, disk config.DataDiskConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createDataDiskCalls = append(m.createDataDiskCalls, disk)
	return m.createDataDiskFunc(vmName, disk)
}

func (m *mockStorageManager) WriteCloudInitISO(cfg *config.VMConfig, isoData []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeCloudInitISOCalls = append(m.writeCloudInitISOCalls, cfg)
	return m.writeCloudInitISOFunc(cfg, isoData)
}

func (m *mockStorageManager) DeleteVM(vmName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteVMCalls = append(m.deleteVMCalls, vmName)
	return m.deleteVMFunc(vmName)
}
