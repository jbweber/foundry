package vm

import (
	"context"
	"fmt"
	"sync"

	"github.com/digitalocean/go-libvirt"

	"github.com/jbweber/foundry/internal/storage"
)

// mockLibvirtClient is a mock implementation of the libvirtClient interface for testing.
type mockLibvirtClient struct {
	mu sync.Mutex

	// Configurable behavior
	domainLookupByNameFunc  func(name string) (libvirt.Domain, error)
	domainDefineXMLFunc     func(xml string) (libvirt.Domain, error)
	domainSetAutostartFunc  func(dom libvirt.Domain, autostart int32) error
	domainCreateFunc        func(dom libvirt.Domain) error
	domainGetStateFunc      func(dom libvirt.Domain, flags uint32) (int32, int32, error)
	domainShutdownFunc      func(dom libvirt.Domain) error
	domainDestroyFunc       func(dom libvirt.Domain) error
	domainUndefineFlagsFunc func(dom libvirt.Domain, flags libvirt.DomainUndefineFlagsValues) error
	domainUndefineFunc      func(dom libvirt.Domain) error

	// Call tracking
	domainLookupByNameCalls  []string
	domainDefineXMLCalls     []string
	domainSetAutostartCalls  []libvirt.Domain
	domainCreateCalls        []libvirt.Domain
	domainGetStateCalls      []libvirt.Domain
	domainShutdownCalls      []libvirt.Domain
	domainDestroyCalls       []libvirt.Domain
	domainUndefineFlagsCalls []libvirt.Domain
	domainUndefineCalls      []libvirt.Domain
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

	// Default: domain state is running
	m.domainGetStateFunc = func(dom libvirt.Domain, flags uint32) (int32, int32, error) {
		return 1, 0, nil // VIR_DOMAIN_RUNNING = 1
	}

	// Default: shutdown succeeds
	m.domainShutdownFunc = func(dom libvirt.Domain) error {
		return nil
	}

	// Default: destroy succeeds
	m.domainDestroyFunc = func(dom libvirt.Domain) error {
		return nil
	}

	// Default: undefine with flags succeeds
	m.domainUndefineFlagsFunc = func(dom libvirt.Domain, flags libvirt.DomainUndefineFlagsValues) error {
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

func (m *mockLibvirtClient) DomainGetState(dom libvirt.Domain, flags uint32) (int32, int32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainGetStateCalls = append(m.domainGetStateCalls, dom)
	return m.domainGetStateFunc(dom, flags)
}

func (m *mockLibvirtClient) DomainShutdown(dom libvirt.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainShutdownCalls = append(m.domainShutdownCalls, dom)
	return m.domainShutdownFunc(dom)
}

func (m *mockLibvirtClient) DomainDestroy(dom libvirt.Domain) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainDestroyCalls = append(m.domainDestroyCalls, dom)
	return m.domainDestroyFunc(dom)
}

func (m *mockLibvirtClient) DomainUndefineFlags(dom libvirt.Domain, flags libvirt.DomainUndefineFlagsValues) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.domainUndefineFlagsCalls = append(m.domainUndefineFlagsCalls, dom)
	return m.domainUndefineFlagsFunc(dom, flags)
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
	ensureDefaultPoolsFunc func(ctx context.Context) error
	volumeExistsFunc       func(ctx context.Context, poolName, volumeName string) (bool, error)
	createVolumeFunc       func(ctx context.Context, poolName string, spec storage.VolumeSpec) error
	deleteVolumeFunc       func(ctx context.Context, poolName, volumeName string) error
	getImagePathFunc       func(ctx context.Context, imageName string) (string, error)
	imageExistsFunc        func(ctx context.Context, imageName string) (bool, error)
	writeVolumeDataFunc    func(ctx context.Context, poolName, volumeName string, data []byte) error
	listVolumesFunc        func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error)

	// Call tracking
	ensureDefaultPoolsCalls int
	volumeExistsCalls       []string // format: "pool/volume"
	createVolumeCalls       []storage.VolumeSpec
	deleteVolumeCalls       []string // format: "pool/volume"
	getImagePathCalls       []string
	imageExistsCalls        []string
	writeVolumeDataCalls    []string // format: "pool/volume"
	listVolumesCalls        []string // pool names
}

// newMockStorageManager creates a new mock storage manager with default behavior.
func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{
		// Default: pools exist
		ensureDefaultPoolsFunc: func(ctx context.Context) error {
			return nil
		},
		// Default: volumes don't exist
		volumeExistsFunc: func(ctx context.Context, poolName, volumeName string) (bool, error) {
			return false, nil
		},
		// Default: create succeeds
		createVolumeFunc: func(ctx context.Context, poolName string, spec storage.VolumeSpec) error {
			return nil
		},
		// Default: delete succeeds
		deleteVolumeFunc: func(ctx context.Context, poolName, volumeName string) error {
			return nil
		},
		// Default: image exists with path
		getImagePathFunc: func(ctx context.Context, imageName string) (string, error) {
			return "/var/lib/libvirt/images/foundry/foundry-images/" + imageName, nil
		},
		// Default: image exists
		imageExistsFunc: func(ctx context.Context, imageName string) (bool, error) {
			return true, nil
		},
		// Default: write succeeds
		writeVolumeDataFunc: func(ctx context.Context, poolName, volumeName string, data []byte) error {
			return nil
		},
		// Default: no volumes
		listVolumesFunc: func(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
			return []storage.VolumeInfo{}, nil
		},
	}
}

func (m *mockStorageManager) EnsureDefaultPools(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureDefaultPoolsCalls++
	return m.ensureDefaultPoolsFunc(ctx)
}

func (m *mockStorageManager) VolumeExists(ctx context.Context, poolName, volumeName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volumeExistsCalls = append(m.volumeExistsCalls, poolName+"/"+volumeName)
	return m.volumeExistsFunc(ctx, poolName, volumeName)
}

func (m *mockStorageManager) CreateVolume(ctx context.Context, poolName string, spec storage.VolumeSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createVolumeCalls = append(m.createVolumeCalls, spec)
	return m.createVolumeFunc(ctx, poolName, spec)
}

func (m *mockStorageManager) DeleteVolume(ctx context.Context, poolName, volumeName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteVolumeCalls = append(m.deleteVolumeCalls, poolName+"/"+volumeName)
	return m.deleteVolumeFunc(ctx, poolName, volumeName)
}

func (m *mockStorageManager) GetImagePath(ctx context.Context, imageName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getImagePathCalls = append(m.getImagePathCalls, imageName)
	return m.getImagePathFunc(ctx, imageName)
}

func (m *mockStorageManager) ImageExists(ctx context.Context, imageName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.imageExistsCalls = append(m.imageExistsCalls, imageName)
	return m.imageExistsFunc(ctx, imageName)
}

func (m *mockStorageManager) WriteVolumeData(ctx context.Context, poolName, volumeName string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeVolumeDataCalls = append(m.writeVolumeDataCalls, poolName+"/"+volumeName)
	return m.writeVolumeDataFunc(ctx, poolName, volumeName, data)
}

func (m *mockStorageManager) ListVolumes(ctx context.Context, poolName string) ([]storage.VolumeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listVolumesCalls = append(m.listVolumesCalls, poolName)
	return m.listVolumesFunc(ctx, poolName)
}
