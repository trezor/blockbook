//go:build unittest

package api

import (
	"strconv"
	"testing"
)

func TestComputeAccountPagingWindows(t *testing.T) {
	tests := []struct {
		name                               string
		mempool, confirmed                 int
		page, itemsOnPage                  int
		wantMempoolFrom, wantMempoolTo     int
		wantConfirmedFrom, wantConfirmedTo int
		wantPage, wantTotalPages           int
	}{
		{
			name:    "first page is shortened by the mempool entries it carries",
			mempool: 1, confirmed: 50, page: 0, itemsOnPage: 25,
			wantMempoolFrom: 0, wantMempoolTo: 1,
			wantConfirmedFrom: 0, wantConfirmedTo: 24,
			wantPage: 1, wantTotalPages: 3,
		},
		{
			name:    "second page resumes where the first one stopped",
			mempool: 1, confirmed: 50, page: 1, itemsOnPage: 25,
			wantMempoolFrom: 1, wantMempoolTo: 1,
			wantConfirmedFrom: 24, wantConfirmedTo: 49,
			wantPage: 2, wantTotalPages: 3,
		},
		{
			name:    "no mempool entries keeps plain confirmed paging",
			mempool: 0, confirmed: 50, page: 1, itemsOnPage: 25,
			wantMempoolFrom: 0, wantMempoolTo: 0,
			wantConfirmedFrom: 25, wantConfirmedTo: 50,
			wantPage: 2, wantTotalPages: 2,
		},
		{
			name:    "more mempool entries than fit on a page spill to the next page",
			mempool: 4, confirmed: 2, page: 1, itemsOnPage: 3,
			wantMempoolFrom: 3, wantMempoolTo: 4,
			wantConfirmedFrom: 0, wantConfirmedTo: 2,
			wantPage: 2, wantTotalPages: 2,
		},
		{
			name:    "page beyond the last one is clamped to the last page",
			mempool: 1, confirmed: 3, page: 9, itemsOnPage: 2,
			wantMempoolFrom: 1, wantMempoolTo: 1,
			wantConfirmedFrom: 1, wantConfirmedTo: 3,
			wantPage: 2, wantTotalPages: 2,
		},
		{
			name:    "empty account has a single empty page",
			mempool: 0, confirmed: 0, page: 0, itemsOnPage: 25,
			wantMempoolFrom: 0, wantMempoolTo: 0,
			wantConfirmedFrom: 0, wantConfirmedTo: 0,
			wantPage: 1, wantTotalPages: 1,
		},
		{
			name:    "mempool only account",
			mempool: 2, confirmed: 0, page: 0, itemsOnPage: 25,
			wantMempoolFrom: 0, wantMempoolTo: 2,
			wantConfirmedFrom: 0, wantConfirmedTo: 0,
			wantPage: 1, wantTotalPages: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pg, mFrom, mTo, cFrom, cTo, _ := computeAccountPaging(tt.mempool, tt.confirmed, tt.page, tt.itemsOnPage)
			if mFrom != tt.wantMempoolFrom || mTo != tt.wantMempoolTo {
				t.Errorf("mempool window = [%d:%d], want [%d:%d]", mFrom, mTo, tt.wantMempoolFrom, tt.wantMempoolTo)
			}
			if cFrom != tt.wantConfirmedFrom || cTo != tt.wantConfirmedTo {
				t.Errorf("confirmed window = [%d:%d], want [%d:%d]", cFrom, cTo, tt.wantConfirmedFrom, tt.wantConfirmedTo)
			}
			if pg.Page != tt.wantPage || pg.TotalPages != tt.wantTotalPages || pg.ItemsOnPage != tt.itemsOnPage {
				t.Errorf("paging = %+v, want page %d of %d, %d items on page", pg, tt.wantPage, tt.wantTotalPages, tt.itemsOnPage)
			}
			if got := (mTo - mFrom) + (cTo - cFrom); got > tt.itemsOnPage {
				t.Errorf("page holds %d items, more than the requested %d", got, tt.itemsOnPage)
			}
		})
	}
}

// TestComputeAccountPagingWalksWholeSequence is the invariant issue #1099 is about: a client
// walking every page must see each tx exactly once, in order, and never get an oversized page.
func TestComputeAccountPagingWalksWholeSequence(t *testing.T) {
	for mempool := 0; mempool <= 5; mempool++ {
		for confirmed := 0; confirmed <= 12; confirmed++ {
			for itemsOnPage := 1; itemsOnPage <= 5; itemsOnPage++ {
				var walked []string
				pg, _, _, _, _, _ := computeAccountPaging(mempool, confirmed, 0, itemsOnPage)
				for page := 0; page < pg.TotalPages; page++ {
					_, mFrom, mTo, cFrom, cTo, normalizedPage := computeAccountPaging(mempool, confirmed, page, itemsOnPage)
					if normalizedPage != page {
						t.Fatalf("mempool=%d confirmed=%d itemsOnPage=%d: page %d was clamped to %d within TotalPages=%d",
							mempool, confirmed, itemsOnPage, page, normalizedPage, pg.TotalPages)
					}
					if got := (mTo - mFrom) + (cTo - cFrom); got > itemsOnPage {
						t.Fatalf("mempool=%d confirmed=%d itemsOnPage=%d page=%d: page holds %d items",
							mempool, confirmed, itemsOnPage, page, got)
					}
					for i := mFrom; i < mTo; i++ {
						walked = append(walked, "m"+strconv.Itoa(i))
					}
					for i := cFrom; i < cTo; i++ {
						walked = append(walked, "c"+strconv.Itoa(i))
					}
				}
				var want []string
				for i := 0; i < mempool; i++ {
					want = append(want, "m"+strconv.Itoa(i))
				}
				for i := 0; i < confirmed; i++ {
					want = append(want, "c"+strconv.Itoa(i))
				}
				if len(walked) != len(want) {
					t.Fatalf("mempool=%d confirmed=%d itemsOnPage=%d: walked %v, want %v", mempool, confirmed, itemsOnPage, walked, want)
				}
				for i := range want {
					if walked[i] != want[i] {
						t.Fatalf("mempool=%d confirmed=%d itemsOnPage=%d: walked %v, want %v", mempool, confirmed, itemsOnPage, walked, want)
					}
				}
			}
		}
	}
}
