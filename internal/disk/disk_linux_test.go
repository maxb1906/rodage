package disk

import (
	"testing"
	"unsafe"
)

func TestStructSizes(t *testing.T) {
	if s := unsafe.Sizeof(sgIOHdr{}); s != 88 {
		t.Fatalf("sg_io_hdr %d", s)
	}
	if s := unsafe.Sizeof(nvmeAdminCmd{}); s != 72 {
		t.Fatalf("nvme_admin_cmd %d", s)
	}
}
