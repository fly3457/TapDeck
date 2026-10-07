package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tapdeck/internal/config"
)

type pairedRecord struct {
	Hash            string    `json:"token_hash"`
	Name            string    `json:"name"`
	PairedAt        time.Time `json:"paired_at"`
	LastConnectedAt time.Time `json:"last_connected_at"`
}
type pairedFile struct {
	SchemaVersion int                     `json:"schema_version"`
	Devices       map[string]pairedRecord `json:"devices"`
}

// PairedDevice is a credential-free view for the local settings window.
type PairedDevice struct {
	ID, Name                  string
	Online                    bool
	PairedAt, LastConnectedAt time.Time
}

func legacyName(id string) string {
	if len(id) > 8 {
		id = id[len(id)-8:]
	}
	return "旧设备（" + id + "）"
}
func savePaired(dir string, records map[string]pairedRecord) error {
	b, err := json.MarshalIndent(pairedFile{SchemaVersion: 2, Devices: records}, "", "  ")
	if err != nil {
		return err
	}
	return config.AtomicWrite(filepath.Join(dir, "paired.json"), b)
}
func loadPaired(dir string) (map[string]pairedRecord, error) {
	records := map[string]pairedRecord{}
	b, err := os.ReadFile(filepath.Join(dir, "paired.json"))
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if _, current := fields["schema_version"]; current {
		var f pairedFile
		if err = json.Unmarshal(b, &f); err != nil {
			return nil, err
		}
		if f.SchemaVersion != 2 || f.Devices == nil {
			return nil, fmt.Errorf("不支持的配对文件格式")
		}
		return f.Devices, nil
	}
	var legacy map[string]string
	if err = json.Unmarshal(b, &legacy); err != nil {
		return nil, err
	}
	for id, hash := range legacy {
		records[id] = pairedRecord{Hash: hash, Name: legacyName(id)}
	}
	backup, err := os.OpenFile(filepath.Join(dir, "paired.v1.bak"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
	if os.IsExist(err) {
		info, statErr := os.Stat(filepath.Join(dir, "paired.v1.bak"))
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("配对备份路径不是普通文件")
		}
	}
	if err == nil {
		_, err = backup.Write(b)
		if err == nil {
			err = backup.Sync()
		}
		closeErr := backup.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	if err = savePaired(dir, records); err != nil {
		return nil, err
	}
	return records, nil
}
func clonePaired(records map[string]pairedRecord) map[string]pairedRecord {
	next := make(map[string]pairedRecord, len(records))
	for id, record := range records {
		next[id] = record
	}
	return next
}
func (s *Server) pairedDevicesLocked() []PairedDevice {
	devices := make([]PairedDevice, 0, len(s.paired))
	for id, record := range s.paired {
		name := record.Name
		if name == "" {
			name = legacyName(id)
		}
		device := PairedDevice{ID: id, Name: name, PairedAt: record.PairedAt, LastConnectedAt: record.LastConnectedAt}
		for _, ss := range s.sessions {
			if ss.deviceID == id {
				device.Online = true
				break
			}
		}
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name != devices[j].Name {
			return devices[i].Name < devices[j].Name
		}
		return devices[i].ID < devices[j].ID
	})
	return devices
}
func (s *Server) PairedDevices() []PairedDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pairedDevicesLocked()
}

func (s *Server) UnpairDevice(id string) error { return s.unpair(id, false) }
func (s *Server) Unpair() error                { return s.unpair("", true) }
func (s *Server) unpair(id string, all bool) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if _, found := s.paired[id]; !all && !found {
		s.mu.Unlock()
		return fmt.Errorf("设备已不在配对列表中")
	}
	next := clonePaired(s.paired)
	if all {
		next = map[string]pairedRecord{}
	} else {
		delete(next, id)
	}
	if err := savePaired(s.dir, next); err != nil {
		s.mu.Unlock()
		return err
	}
	s.paired = next
	if all {
		s.generation++
	} else {
		s.pairEpoch[id]++
	}
	var sessions []*session
	var pending []*Pending
	for sid, ss := range s.sessions {
		if all || ss.deviceID == id {
			sessions = append(sessions, ss)
			delete(s.sessions, sid)
		}
	}
	for requestID, p := range s.pending {
		if all || p.DeviceID == id {
			pending = append(pending, p)
			delete(s.pending, requestID)
		}
	}
	s.mu.Unlock()
	for _, ss := range sessions {
		_ = ss.send(map[string]any{"type": "error", "code": "pairing_revoked", "reason": "电脑已解除此设备配对，请重新连接并由电脑允许"})
		ss.close("已解除配对")
	}
	for _, p := range pending {
		if p.revoke != nil {
			p.revoke()
		} else if p.cancel != nil {
			p.cancel()
		}
	}
	return nil
}
