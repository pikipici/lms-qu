package kelas

func toKelasListItems(rows []Kelas) []KelasListItem {
	items := make([]KelasListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, KelasListItem{
			Kelas:       row,
			JumlahMurid: row.JumlahMurid,
		})
	}
	return items
}
