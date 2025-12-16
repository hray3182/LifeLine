package models

type Category struct {
	CategoryID   int    `json:"category_id"`
	UserID       int64  `json:"user_id"`
	CategoryName string `json:"category_name"`
	UsageCount   int    `json:"usage_count"`
}

type Subcategory struct {
	SubcategoryID   int    `json:"subcategory_id"`
	CategoryID      int    `json:"category_id"`
	SubcategoryName string `json:"subcategory_name"`
	UsageCount      int    `json:"usage_count"`
}
