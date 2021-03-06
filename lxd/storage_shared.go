package main

import (
	"fmt"
	"os/exec"

	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
)

type storageShared struct {
	sType        storageType
	sTypeName    string
	sTypeVersion string

	d *Daemon

	poolID int64
	pool   *api.StoragePool

	volume *api.StorageVolume
}

func (s *storageShared) GetStorageType() storageType {
	return s.sType
}

func (s *storageShared) GetStorageTypeName() string {
	return s.sTypeName
}

func (s *storageShared) GetStorageTypeVersion() string {
	return s.sTypeVersion
}

func (s *storageShared) shiftRootfs(c container) error {
	dpath := c.Path()
	rpath := c.RootfsPath()

	shared.LogDebugf("Shifting root filesystem \"%s\" for \"%s\".", rpath, c.Name())

	idmapset, err := c.IdmapSet()
	if err != nil {
		return err
	}

	if idmapset == nil {
		return fmt.Errorf("IdmapSet of container '%s' is nil", c.Name())
	}

	err = idmapset.ShiftRootfs(rpath)
	if err != nil {
		shared.LogDebugf("Shift of rootfs %s failed: %s", rpath, err)
		return err
	}

	/* Set an acl so the container root can descend the container dir */
	// TODO: i changed this so it calls s.setUnprivUserAcl, which does
	// the acl change only if the container is not privileged, think thats right.
	return s.setUnprivUserAcl(c, dpath)
}

func (s *storageShared) setUnprivUserAcl(c container, destPath string) error {
	idmapset, err := c.IdmapSet()
	if err != nil {
		return err
	}

	// Skip for privileged containers
	if idmapset == nil {
		return nil
	}

	// Make sure the map is valid. Skip if container uid 0 == host uid 0
	uid, _ := idmapset.ShiftIntoNs(0, 0)
	switch uid {
	case -1:
		return fmt.Errorf("Container doesn't have a uid 0 in its map")
	case 0:
		return nil
	}

	// Attempt to set a POSIX ACL first.
	acl := fmt.Sprintf("%d:rx", uid)
	err = exec.Command("setfacl", "-m", acl, destPath).Run()
	if err == nil {
		return nil
	}

	// Fallback to chmod if the fs doesn't support it.
	err = exec.Command("chmod", "+x", destPath).Run()
	if err != nil {
		shared.LogDebugf("Failed to set executable bit on the container path: %s", err)
		return err
	}

	return nil
}

func (s *storageShared) createImageDbPoolVolume(fingerprint string) error {
	// Fill in any default volume config.
	volumeConfig := map[string]string{}
	err := storageVolumeFillDefault(s.pool.Name, volumeConfig, s.pool)
	if err != nil {
		return err
	}

	// Create a db entry for the storage volume of the image.
	_, err = dbStoragePoolVolumeCreate(s.d.db, fingerprint, storagePoolVolumeTypeImage, s.poolID, volumeConfig)
	if err != nil {
		// Try to delete the db entry on error.
		s.deleteImageDbPoolVolume(fingerprint)
		return err
	}

	return nil
}

func (s *storageShared) deleteImageDbPoolVolume(fingerprint string) error {
	err := dbStoragePoolVolumeDelete(s.d.db, fingerprint, storagePoolVolumeTypeImage, s.poolID)
	if err != nil {
		return err
	}

	return nil
}
