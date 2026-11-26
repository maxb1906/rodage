package sysinfo

// parseSMBIOS lit une table SMBIOS brute (structures enchaînées) et remplit
// les types 0 (BIOS), 1 (système) et 2 (carte mère).
func parseSMBIOS(tbl []byte) SMBIOS {
	var s SMBIOS
	for off := 0; off+4 <= len(tbl); {
		typ := tbl[off]
		ln := int(tbl[off+1])
		if ln < 4 || off+ln > len(tbl) {
			break
		}
		hdr := tbl[off : off+ln]
		// Chaînes : à la suite de la partie formatée, terminées par un double zéro.
		strs := []string{}
		p := off + ln
		start := p
		for p+1 < len(tbl) {
			if tbl[p] == 0 {
				if p > start {
					strs = append(strs, string(tbl[start:p]))
				}
				if tbl[p+1] == 0 {
					p += 2
					break
				}
				start = p + 1
			}
			p++
		}
		str := func(idx int) string {
			if idx >= len(hdr) {
				return ""
			}
			i := int(hdr[idx])
			if i == 0 || i > len(strs) {
				return ""
			}
			return clean(strs[i-1])
		}
		switch typ {
		case 0:
			s.BIOSVendor, s.BIOSVersion, s.BIOSDate = str(4), str(5), str(8)
		case 1:
			s.Manufacturer, s.Product, s.Serial = str(4), str(5), str(7)
		case 2:
			s.BoardVendor, s.BoardProduct, s.BoardSerial = str(4), str(5), str(7)
		case 127:
			return s
		}
		off = p
	}
	return s
}
