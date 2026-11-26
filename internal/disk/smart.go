package disk

import "encoding/binary"

// parseNVMeHealth décode le journal SMART / Health Information (page 02h).
func parseNVMeHealth(b []byte) Health {
	h := unknownHealth()
	if len(b) < 512 {
		h.Note = "journal NVMe incomplet"
		return h
	}
	u64 := func(off int) uint64 { return binary.LittleEndian.Uint64(b[off:]) } // 128 bits : partie basse
	h.Available = true
	h.Protocol = "nvme"
	h.CriticalWarning = int(b[0])
	if k := binary.LittleEndian.Uint16(b[1:]); k > 0 {
		h.TempC = float64(k) - 273.15
	}
	h.AvailableSpare = int(b[3])
	h.PercentUsed = int(b[5])
	h.DataRead = u64(32) * 512000
	h.DataWritten = u64(48) * 512000
	h.PowerCycles = int64(u64(112))
	h.PowerOnHours = int64(u64(128))
	h.UnsafeShutdowns = int64(u64(144))
	h.MediaErrors = int64(u64(160))
	h.ErrorLogEntries = int64(u64(176))
	return h
}

// parseATASmart décode la table d'attributs SMART ATA (512 octets).
func parseATASmart(b []byte) Health {
	h := unknownHealth()
	if len(b) < 362 {
		h.Note = "table SMART incomplète"
		return h
	}
	h.Available = true
	h.Protocol = "ata"
	for i := 0; i < 30; i++ {
		a := b[2+12*i : 2+12*(i+1)]
		id := a[0]
		if id == 0 {
			continue
		}
		raw := uint64(a[5]) | uint64(a[6])<<8 | uint64(a[7])<<16 | uint64(a[8])<<24 | uint64(a[9])<<32 | uint64(a[10])<<40
		low32 := int64(raw & 0xFFFFFFFF)
		switch id {
		case 5:
			h.Reallocated = low32
		case 9:
			h.PowerOnHours = low32
		case 12:
			h.PowerCycles = low32
		case 187:
			h.Uncorrectable = low32
		case 194:
			h.TempC = float64(raw & 0xFF)
		case 190:
			if h.TempC < 0 {
				h.TempC = float64(raw & 0xFF)
			}
		case 197:
			h.Pending = low32
		case 198:
			if h.Uncorrectable < 0 {
				h.Uncorrectable = low32
			} else {
				h.Uncorrectable += low32
			}
		case 199:
			h.CRCErrors = low32
		case 192:
			h.UnsafeShutdowns = low32
		case 241:
			h.DataWritten = raw * 512
		case 242:
			h.DataRead = raw * 512
		case 231:
			// Durée de vie restante (SSD) : 100 = neuf.
			if v := int(a[3]); v <= 100 {
				h.PercentUsed = 100 - v
			}
		}
	}
	return h
}
