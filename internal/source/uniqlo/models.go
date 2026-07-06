package uniqlo

import "encoding/json"

// productsResponse is the top-level shape of both the listing endpoint and
// the "single product by id" endpoint.
type productsResponse struct {
	Status string  `json:"status"`
	Result *result `json:"result"`
}

type result struct {
	Items      []item     `json:"items"`
	Pagination pagination `json:"pagination"`
}

type pagination struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Count  int `json:"count"`
}

// item is one product tile returned by the listing endpoint.
type item struct {
	ProductID  string  `json:"productId"`
	L1ID       string  `json:"l1Id"`
	Name       string  `json:"name"`
	Prices     prices  `json:"prices"`
	Colors     []color `json:"colors"`
	Sizes      []size  `json:"sizes"`
	PriceGroup string  `json:"priceGroup"`
}

type prices struct {
	Base               *priceValue         `json:"base"`
	Promo              *priceValue         `json:"promo"`
	IsDualPrice        bool                `json:"isDualPrice"`
	DiscountPercentage *float64            `json:"discountPercentage"`
	LowestPriceDetails *lowestPriceDetails `json:"lowestPriceDetails"`
}

type priceValue struct {
	Value    float64  `json:"value"`
	Currency currency `json:"currency"`
}

type currency struct {
	Code   string `json:"code"`
	Symbol string `json:"symbol"`
}

type lowestPriceDetails struct {
	CanDisplayLowestPrice bool    `json:"canDisplayLowestPrice"`
	LowestPeriod          int     `json:"lowestPeriod"`
	LowestPrice           float64 `json:"lowestPrice"`
}

type color struct {
	Code        string `json:"code"`
	DisplayCode string `json:"displayCode"`
	Name        string `json:"name"`
}

type size struct {
	Code        string `json:"code"`
	DisplayCode string `json:"displayCode"`
	Name        string `json:"name"`
	// Display in the real API is an object ({showFlag, chipType}); we
	// deliberately do not decode its shape because we don't use it. A raw
	// message keeps decoding tolerant of future field additions.
	Display json.RawMessage `json:"display,omitempty"`
}

// l2sResponse is the shape of the l2s (per-color/size stock) endpoint.
type l2sResponse struct {
	Status string  `json:"status"`
	Result *l2Data `json:"result"`
}

type l2Data struct {
	L2s []l2 `json:"l2s"`
}

// l2 is a size-and-color line item.
type l2 struct {
	L2ID              string      `json:"l2Id"`
	Color             color       `json:"color"`
	Size              size        `json:"size"`
	Sales             bool        `json:"sales"`
	SalesType         string      `json:"salesType"`
	StockStatusCode   string      `json:"stockStatusCode"`
	CommunicationCode string      `json:"communicationCode"`
	Prices            *pricesFlat `json:"prices"`
}

// pricesFlat is a stripped price block on l2 rows (fields we don't need are omitted).
type pricesFlat struct {
	Base  *priceValue `json:"base"`
	Promo *priceValue `json:"promo"`
}
