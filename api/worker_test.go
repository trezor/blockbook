//go:build unittest

package api

import (
	"testing"
)

func Test_computePaging(t *testing.T) {
	tests := []struct {
		name           string
		count          int
		page           int
		itemsOnPage    int
		wantPage       int
		wantPagingPage int
		wantFrom       int
		wantTo         int
		wantTotalPages int
	}{
		{"normal case", 100, 0, 10, 0, 1, 0, 10, 10},
		{"second page", 100, 1, 10, 1, 2, 10, 20, 10},
		{"last page", 100, 9, 10, 9, 10, 90, 100, 10},
		{"page beyond count", 100, 20, 10, 9, 10, 90, 100, 10},
		{"zero count", 0, 0, 10, 0, 1, 0, 0, 1},
		{"negative page", 100, -5, 10, 0, 1, 0, 10, 10},
		{"negative itemsOnPage", 100, 0, -5, 0, 1, 0, 1, 100},
		{"zero itemsOnPage", 100, 0, 0, 0, 1, 0, 1, 100},
		{"negative count", -10, 0, 10, 0, 1, 0, 0, 1},
		{"overflow protection - large page", 1000, 100000000, 1000, 0, 1, 0, 1000, 1},
		{"overflow protection - large itemsOnPage", 1000, 100, 10000000, 0, 1, 0, 1000, 1},
		{"overflow protection - both large", 1000, 1000000, 10000, 0, 1, 0, 1000, 1},
		{"exact fit", 50, 4, 10, 4, 5, 40, 50, 5},
		{"remainder", 55, 5, 10, 5, 6, 50, 55, 6},
		{"single item", 1, 0, 10, 0, 1, 0, 1, 1},
		{"page 0 with items", 25, 0, 5, 0, 1, 0, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPaging, gotFrom, gotTo, gotPage := computePaging(tt.count, tt.page, tt.itemsOnPage)
			if gotPage != tt.wantPage {
				t.Errorf("computePaging() page = %v, want %v", gotPage, tt.wantPage)
			}
			if gotFrom != tt.wantFrom {
				t.Errorf("computePaging() from = %v, want %v", gotFrom, tt.wantFrom)
			}
			if gotTo != tt.wantTo {
				t.Errorf("computePaging() to = %v, want %v", gotTo, tt.wantTo)
			}
			if gotPaging.Page != tt.wantPagingPage {
				t.Errorf("computePaging() Paging.Page = %v, want %v", gotPaging.Page, tt.wantPagingPage)
			}
			if gotPaging.TotalPages != tt.wantTotalPages {
				t.Errorf("computePaging() Paging.TotalPages = %v, want %v", gotPaging.TotalPages, tt.wantTotalPages)
			}
			if gotPaging.ItemsOnPage != tt.itemsOnPage {
				if tt.itemsOnPage <= 0 {
					if gotPaging.ItemsOnPage != 1 {
						t.Errorf("computePaging() Paging.ItemsOnPage = %v, want 1 (sanitized)", gotPaging.ItemsOnPage)
					}
				} else {
					t.Errorf("computePaging() Paging.ItemsOnPage = %v, want %v", gotPaging.ItemsOnPage, tt.itemsOnPage)
				}
			}
		})
	}
}
