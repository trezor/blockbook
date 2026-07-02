package db

import (
	"encoding/json"
	"time"

	"github.com/juju/errors"
)

// runtimeSettingKeyPrefix prefixes cfDefault keys holding runtime setting
// overrides written through the internal /admin interface. Each setting is
// stored under its own key, so a write or delete touches nothing else in the
// database (in particular it is independent of the periodically stored
// internalState blob) and the rows are ignored by older blockbook versions.
const runtimeSettingKeyPrefix = "runtimeSetting:"

// runtimeSettingEnvelope wraps a stored runtime setting value. The envelope
// keeps a stored empty value distinguishable from a missing row and records
// when the value was last changed.
type runtimeSettingEnvelope struct {
	Value   string    `json:"value"`
	Updated time.Time `json:"updated"`
}

// GetRuntimeSetting returns the stored override of the given runtime setting
// and whether an override exists.
func (d *RocksDB) GetRuntimeSetting(name string) (string, bool, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(runtimeSettingKeyPrefix+name))
	if err != nil {
		return "", false, err
	}
	defer val.Free()
	data := val.Data()
	if len(data) == 0 {
		return "", false, nil
	}
	var e runtimeSettingEnvelope
	if err := json.Unmarshal(data, &e); err != nil {
		return "", false, errors.Annotatef(err, "cannot unpack runtime setting %s", name)
	}
	return e.Value, true, nil
}

// StoreRuntimeSetting persists an override of the given runtime setting.
func (d *RocksDB) StoreRuntimeSetting(name, value string) error {
	buf, err := json.Marshal(&runtimeSettingEnvelope{Value: value, Updated: time.Now()})
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(runtimeSettingKeyPrefix+name), buf)
}

// DeleteRuntimeSetting removes the override of the given runtime setting.
func (d *RocksDB) DeleteRuntimeSetting(name string) error {
	return d.db.DeleteCF(d.wo, d.cfh[cfDefault], []byte(runtimeSettingKeyPrefix+name))
}
