package storage

import (
	"fmt"
	"io"
	"strings"

	"github.com/digitalocean/go-libvirt"
)

// mockLibvirtClient is a mock implementation of LibvirtClient for testing.
type mockLibvirtClient struct {
	pools   map[string]*mockPool
	volumes map[string]map[string]*mockVolume // pool name -> volume name -> volume
}

type mockPool struct {
	name      string
	uuid      string
	state     libvirt.StoragePoolState
	capacity  uint64
	allocated uint64
	available uint64
	xmlDesc   string
}

type mockVolume struct {
	name      string
	path      string
	capacity  uint64
	allocated uint64
	data      []byte
}

func newMockLibvirtClient() *mockLibvirtClient {
	return &mockLibvirtClient{
		pools:   make(map[string]*mockPool),
		volumes: make(map[string]map[string]*mockVolume),
	}
}

func (m *mockLibvirtClient) StoragePoolLookupByName(name string) (libvirt.StoragePool, error) {
	pool, ok := m.pools[name]
	if !ok {
		return libvirt.StoragePool{}, fmt.Errorf("storage pool not found: %s", name)
	}
	// Convert string UUID to libvirt.UUID (16-byte array)
	var uuid libvirt.UUID
	copy(uuid[:], pool.uuid)
	return libvirt.StoragePool{
		Name: pool.name,
		UUID: uuid,
	}, nil
}

func (m *mockLibvirtClient) StoragePoolDefineXML(xml string, flags uint32) (libvirt.StoragePool, error) {
	// Parse pool name from XML
	name := extractTagValue(xml, "name")
	if name == "" {
		return libvirt.StoragePool{}, fmt.Errorf("invalid pool XML: missing name")
	}

	// Check if pool already exists
	if _, ok := m.pools[name]; ok {
		return libvirt.StoragePool{}, fmt.Errorf("storage pool already exists: %s", name)
	}

	// Create pool
	pool := &mockPool{
		name:      name,
		uuid:      "mock-uuid-" + name,
		state:     libvirt.StoragePoolInactive,
		capacity:  1024 * 1024 * 1024 * 1024, // 1 TB
		allocated: 0,
		available: 1024 * 1024 * 1024 * 1024, // 1 TB
		xmlDesc:   xml,
	}
	m.pools[name] = pool
	m.volumes[name] = make(map[string]*mockVolume)

	// Convert string UUID to libvirt.UUID (16-byte array)
	var uuid libvirt.UUID
	copy(uuid[:], pool.uuid)
	return libvirt.StoragePool{
		Name: pool.name,
		UUID: uuid,
	}, nil
}

func (m *mockLibvirtClient) StoragePoolCreate(pool libvirt.StoragePool, flags libvirt.StoragePoolCreateFlags) error {
	p, ok := m.pools[pool.Name]
	if !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	p.state = libvirt.StoragePoolRunning
	return nil
}

func (m *mockLibvirtClient) StoragePoolBuild(pool libvirt.StoragePool, flags libvirt.StoragePoolBuildFlags) error {
	if _, ok := m.pools[pool.Name]; !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	return nil
}

func (m *mockLibvirtClient) StoragePoolSetAutostart(pool libvirt.StoragePool, autostart int32) error {
	if _, ok := m.pools[pool.Name]; !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	return nil
}

func (m *mockLibvirtClient) StoragePoolDestroy(pool libvirt.StoragePool) error {
	p, ok := m.pools[pool.Name]
	if !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	p.state = libvirt.StoragePoolInactive
	return nil
}

func (m *mockLibvirtClient) StoragePoolUndefine(pool libvirt.StoragePool) error {
	if _, ok := m.pools[pool.Name]; !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	delete(m.pools, pool.Name)
	delete(m.volumes, pool.Name)
	return nil
}

func (m *mockLibvirtClient) StoragePoolGetInfo(pool libvirt.StoragePool) (rState uint8, rCapacity uint64, rAllocation uint64, rAvailable uint64, err error) {
	p, ok := m.pools[pool.Name]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	return uint8(p.state), p.capacity, p.allocated, p.available, nil
}

func (m *mockLibvirtClient) StoragePoolGetXMLDesc(pool libvirt.StoragePool, flags libvirt.StorageXMLFlags) (string, error) {
	p, ok := m.pools[pool.Name]
	if !ok {
		return "", fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	return p.xmlDesc, nil
}

func (m *mockLibvirtClient) StoragePoolListAllVolumes(pool libvirt.StoragePool, needResults int32, flags uint32) ([]libvirt.StorageVol, uint32, error) {
	vols, ok := m.volumes[pool.Name]
	if !ok {
		return nil, 0, fmt.Errorf("storage pool not found: %s", pool.Name)
	}

	var result []libvirt.StorageVol
	for name := range vols {
		result = append(result, libvirt.StorageVol{
			Pool: pool.Name,
			Name: name,
		})
	}

	return result, uint32(len(result)), nil
}

func (m *mockLibvirtClient) StoragePoolRefresh(pool libvirt.StoragePool, flags uint32) error {
	if _, ok := m.pools[pool.Name]; !ok {
		return fmt.Errorf("storage pool not found: %s", pool.Name)
	}
	return nil
}

func (m *mockLibvirtClient) StorageVolLookupByName(pool libvirt.StoragePool, name string) (libvirt.StorageVol, error) {
	vols, ok := m.volumes[pool.Name]
	if !ok {
		return libvirt.StorageVol{}, fmt.Errorf("storage pool not found: %s", pool.Name)
	}

	vol, ok := vols[name]
	if !ok {
		return libvirt.StorageVol{}, fmt.Errorf("storage volume not found: %s", name)
	}

	return libvirt.StorageVol{
		Pool: pool.Name,
		Name: vol.name,
	}, nil
}

func (m *mockLibvirtClient) StorageVolCreateXML(pool libvirt.StoragePool, xml string, flags libvirt.StorageVolCreateFlags) (libvirt.StorageVol, error) {
	vols, ok := m.volumes[pool.Name]
	if !ok {
		return libvirt.StorageVol{}, fmt.Errorf("storage pool not found: %s", pool.Name)
	}

	// Parse volume name from XML
	name := extractTagValue(xml, "name")
	if name == "" {
		return libvirt.StorageVol{}, fmt.Errorf("invalid volume XML: missing name")
	}

	// Check if volume already exists
	if _, ok := vols[name]; ok {
		return libvirt.StorageVol{}, fmt.Errorf("storage volume already exists: %s", name)
	}

	// Create volume
	vol := &mockVolume{
		name:      name,
		path:      "/var/lib/libvirt/images/foundry/" + pool.Name + "/" + name,
		capacity:  100 * 1024 * 1024 * 1024, // 100 GB default
		allocated: 0,
	}
	vols[name] = vol

	return libvirt.StorageVol{
		Pool: pool.Name,
		Name: vol.name,
	}, nil
}

func (m *mockLibvirtClient) StorageVolDelete(vol libvirt.StorageVol, flags libvirt.StorageVolDeleteFlags) error {
	vols, ok := m.volumes[vol.Pool]
	if !ok {
		return fmt.Errorf("storage pool not found: %s", vol.Pool)
	}

	if _, ok := vols[vol.Name]; !ok {
		return fmt.Errorf("storage volume not found: %s", vol.Name)
	}

	delete(vols, vol.Name)
	return nil
}

func (m *mockLibvirtClient) StorageVolGetPath(vol libvirt.StorageVol) (string, error) {
	vols, ok := m.volumes[vol.Pool]
	if !ok {
		return "", fmt.Errorf("storage pool not found: %s", vol.Pool)
	}

	v, ok := vols[vol.Name]
	if !ok {
		return "", fmt.Errorf("storage volume not found: %s", vol.Name)
	}

	return v.path, nil
}

func (m *mockLibvirtClient) StorageVolGetInfo(vol libvirt.StorageVol) (rType int8, rCapacity uint64, rAllocation uint64, err error) {
	vols, ok := m.volumes[vol.Pool]
	if !ok {
		return 0, 0, 0, fmt.Errorf("storage pool not found: %s", vol.Pool)
	}

	v, ok := vols[vol.Name]
	if !ok {
		return 0, 0, 0, fmt.Errorf("storage volume not found: %s", vol.Name)
	}

	return 0, v.capacity, v.allocated, nil
}

func (m *mockLibvirtClient) StorageVolUpload(vol libvirt.StorageVol, reader io.Reader, offset uint64, length uint64, flags libvirt.StorageVolUploadFlags) error {
	vols, ok := m.volumes[vol.Pool]
	if !ok {
		return fmt.Errorf("storage pool not found: %s", vol.Pool)
	}

	v, ok := vols[vol.Name]
	if !ok {
		return fmt.Errorf("storage volume not found: %s", vol.Name)
	}

	// Read all data from the reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	v.data = data
	v.allocated = uint64(len(data))
	return nil
}

func (m *mockLibvirtClient) ConnectListAllStoragePools(needResults int32, flags libvirt.ConnectListAllStoragePoolsFlags) ([]libvirt.StoragePool, uint32, error) {
	var result []libvirt.StoragePool
	for name, pool := range m.pools {
		// Convert string UUID to libvirt.UUID (16-byte array)
		var uuid libvirt.UUID
		copy(uuid[:], pool.uuid)
		result = append(result, libvirt.StoragePool{
			Name: name,
			UUID: uuid,
		})
	}
	return result, uint32(len(result)), nil
}

// Helper function to extract tag value from XML
func extractTagValue(xml, tag string) string {
	start := strings.Index(xml, "<"+tag+">")
	if start == -1 {
		return ""
	}
	start += len(tag) + 2
	end := strings.Index(xml[start:], "</"+tag+">")
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}
