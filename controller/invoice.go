package controller

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func invoiceIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

func writeInvoiceCSV(c *gin.Context, inv *model.Invoice) {
	filename := fmt.Sprintf("invoice-%s-%s-%d.csv", inv.Period, inv.Scope, inv.Id)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"period", "scope", "reseller_id", "reseller_name", "customer_id", "customer_name",
		"requests", "charge_quota", "cost_quota", "profit_quota",
		"charge_usd", "cost_usd", "profit_usd", "model",
		"model_requests", "model_charge_quota", "model_cost_quota", "model_profit_quota",
		"model_charge_usd", "model_cost_usd", "model_profit_usd",
	})
	items := inv.InvoiceLineItems()
	if len(items) == 0 {
		_ = w.Write([]string{
			inv.Period, inv.Scope,
			strconv.Itoa(inv.ResellerId), inv.ResellerName,
			strconv.Itoa(inv.CustomerId), inv.CustomerName,
			strconv.FormatInt(inv.Requests, 10),
			strconv.FormatInt(inv.ChargeQuota, 10),
			strconv.FormatInt(inv.CostQuota, 10),
			strconv.FormatInt(inv.ProfitQuota, 10),
			fmt.Sprintf("%.6f", inv.ChargeUsd),
			fmt.Sprintf("%.6f", inv.CostUsd),
			fmt.Sprintf("%.6f", inv.ProfitUsd),
			"", "", "", "", "", "", "", "",
		})
	} else {
		for _, it := range items {
			_ = w.Write([]string{
				inv.Period, inv.Scope,
				strconv.Itoa(inv.ResellerId), inv.ResellerName,
				strconv.Itoa(inv.CustomerId), inv.CustomerName,
				strconv.FormatInt(inv.Requests, 10),
				strconv.FormatInt(inv.ChargeQuota, 10),
				strconv.FormatInt(inv.CostQuota, 10),
				strconv.FormatInt(inv.ProfitQuota, 10),
				fmt.Sprintf("%.6f", inv.ChargeUsd),
				fmt.Sprintf("%.6f", inv.CostUsd),
				fmt.Sprintf("%.6f", inv.ProfitUsd),
				it.Model,
				strconv.FormatInt(it.Requests, 10),
				strconv.FormatInt(it.ChargeQuota, 10),
				strconv.FormatInt(it.CostQuota, 10),
				strconv.FormatInt(it.ProfitQuota, 10),
				fmt.Sprintf("%.6f", it.ChargeUsd),
				fmt.Sprintf("%.6f", it.CostUsd),
				fmt.Sprintf("%.6f", it.ProfitUsd),
			})
		}
	}
	w.Flush()
}

func invoiceDetailResponse(inv *model.Invoice) gin.H {
	return gin.H{
		"invoice":    inv,
		"line_items": inv.InvoiceLineItems(),
	}
}

func listInvoicesHandler(c *gin.Context, f model.InvoiceListFilter) {
	pageInfo := common.GetPageQuery(c)
	f.StartIdx = pageInfo.GetStartIdx()
	f.Num = pageInfo.GetPageSize()
	if period := strings.TrimSpace(c.Query("period")); period != "" {
		f.Period = period
	}
	if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
		f.Scope = scope
	}
	if rid, err := strconv.Atoi(c.Query("reseller_id")); err == nil && rid > 0 {
		f.ResellerId = rid
	}
	if cid, err := strconv.Atoi(c.Query("customer_id")); err == nil && cid > 0 {
		f.CustomerId = cid
	}
	rows, total, err := model.ListInvoices(f)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

// AdminListInvoices lists all persisted monthly invoices.
func AdminListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{})
}

// AdminGetInvoice returns one invoice (JSON) or CSV when format=csv.
func AdminGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}

// AdminGenerateInvoices builds monthly invoices from logs for period YYYY-MM.
func AdminGenerateInvoices(c *gin.Context) {
	var req struct {
		Period string `json:"period"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || strings.TrimSpace(req.Period) == "" {
		common.ApiErrorMsg(c, "period is required (YYYY-MM)")
		return
	}
	result, err := model.GenerateMonthlyInvoices(strings.TrimSpace(req.Period), c.GetInt("id"))
	if err != nil {
		if errors.Is(err, model.ErrInvalidInvoicePeriod) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// ResellerListInvoices lists own reseller invoice + customer invoices.
func ResellerListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{RestrictResellerId: c.GetInt("id")})
}

// ResellerGetInvoice returns an invoice belonging to this reseller.
func ResellerGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	rid := c.GetInt("id")
	if inv.ResellerId != rid || (inv.Scope != model.InvoiceScopeCustomer && inv.Scope != model.InvoiceScopeReseller) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}

// CustomerListInvoices lists the caller's own customer invoices.
func CustomerListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{
		Scope:      model.InvoiceScopeCustomer,
		CustomerId: c.GetInt("id"),
	})
}

// CustomerGetInvoice returns the caller's own customer invoice.
func CustomerGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	if inv.Scope != model.InvoiceScopeCustomer || inv.CustomerId != c.GetInt("id") {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}
