package storage

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
)

// KVMetaData is the small, whole-rewritten piece of state that names the current
// SSTable levels, newest first.
type KVMetaData struct {
	Version  uint64
	SSTables []string
}

// KVMetaItem is one on-disk slot: [ crc32 | version | json(data) ], where crc32
// covers everything after it.
type KVMetaItem struct {
	FileName string
	fp       *os.File
	data     KVMetaData
	valid    bool // did this slot load with a good checksum?
}

func (it *KVMetaItem) open() error {
	fp, err := createFileSync(it.FileName)
	if err != nil {
		return err
	}
	it.fp = fp

	buf, err := os.ReadFile(it.FileName)
	if err != nil {
		return err
	}
	if len(buf) < 12 {
		return nil // empty / never written
	}
	want := binary.LittleEndian.Uint32(buf[0:4])
	if crc32.ChecksumIEEE(buf[4:]) != want {
		return nil // corrupt slot — ignore it
	}
	it.data.Version = binary.LittleEndian.Uint64(buf[4:12])
	if err := json.Unmarshal(buf[12:], &it.data); err != nil {
		return nil
	}
	it.valid = true
	return nil
}

func (it *KVMetaItem) write(data KVMetaData) error {
	blob, err := json.Marshal(data)
	if err != nil {
		return err
	}
	body := make([]byte, 8+len(blob))
	binary.LittleEndian.PutUint64(body[0:8], data.Version)
	copy(body[8:], blob)

	out := make([]byte, 4+len(body))
	binary.LittleEndian.PutUint32(out[0:4], crc32.ChecksumIEEE(body))
	copy(out[4:], body)

	if err := it.fp.Truncate(0); err != nil {
		return err
	}
	if _, err := it.fp.WriteAt(out, 0); err != nil {
		return err
	}
	if err := it.fp.Sync(); err != nil {
		return err
	}
	it.data = data
	it.valid = true
	return nil
}

// KVMetaStore persists KVMetaData crash-safely by double buffering: two slots,
// writes alternate, only the older is ever overwritten. After a crash: discard
// any slot with a bad checksum; of the survivors the higher Version is current.
// You can never lose both.
type KVMetaStore struct {
	slots [2]KVMetaItem
}

// Open opens both slots, ignoring a corrupt one.
func (m *KVMetaStore) Open() error {
	for i := range m.slots {
		if err := m.slots[i].open(); err != nil {
			return err
		}
	}
	return nil
}

// Close closes both slots.
func (m *KVMetaStore) Close() error {
	var err error
	for i := range m.slots {
		if m.slots[i].fp != nil {
			if e := m.slots[i].fp.Close(); err == nil {
				err = e
			}
		}
	}
	return err
}

// current returns the index of the slot holding the live data (higher version
// among valid slots), or -1 if neither slot has ever been written.
func (m *KVMetaStore) current() int {
	a, b := m.slots[0].valid, m.slots[1].valid
	switch {
	case a && b:
		if m.slots[0].data.Version >= m.slots[1].data.Version {
			return 0
		}
		return 1
	case a:
		return 0
	case b:
		return 1
	default:
		return -1
	}
}

// Get returns the live metadata (zero value if never written).
func (m *KVMetaStore) Get() KVMetaData {
	if i := m.current(); i >= 0 {
		return m.slots[i].data
	}
	return KVMetaData{}
}

var errMetaVersion = errors.New("meta: new version must exceed the current one")

// Set writes data into the older slot (the one not currently live) and fsyncs.
// data.Version must be greater than the current version.
func (m *KVMetaStore) Set(data KVMetaData) error {
	cur := m.current()
	if cur >= 0 && data.Version <= m.slots[cur].data.Version {
		return errMetaVersion
	}
	target := 0
	if cur == 0 {
		target = 1
	}
	return m.slots[target].write(data)
}
