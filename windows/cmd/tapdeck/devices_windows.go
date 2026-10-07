//go:build windows

package main

import (
	"github.com/lxn/walk"
	"reflect"
	"tapdeck/internal/server"
)

type pairedModel struct {
	walk.TableModelBase
	items []server.PairedDevice
}

func (m *pairedModel) RowCount() int { return len(m.items) }
func (m *pairedModel) Value(row, column int) interface{} {
	v := m.items[row]
	switch column {
	case 0:
		return v.Name
	case 1:
		if v.Online {
			return "在线"
		}
		return "离线"
	case 2:
		return v.ID
	case 3:
		if v.LastConnectedAt.IsZero() {
			return "尚无记录"
		}
		return v.LastConnectedAt.Local().Format("2006-01-02 15:04:05")
	}
	return ""
}
func (m *pairedModel) selected(table *walk.TableView) (server.PairedDevice, bool) {
	i := table.CurrentIndex()
	if i < 0 || i >= len(m.items) {
		return server.PairedDevice{}, false
	}
	return m.items[i], true
}
func (m *pairedModel) update(table *walk.TableView, devices []server.PairedDevice) {
	if reflect.DeepEqual(m.items, devices) {
		return
	}
	selected, _ := m.selected(table)
	m.items = devices
	m.PublishRowsReset()
	index := -1
	for i, v := range devices {
		if v.ID == selected.ID {
			index = i
			break
		}
	}
	_ = table.SetCurrentIndex(index)
}
