/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { SectionPageLayout } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { toIntlLocale } from "@/i18n/languages";
import { api } from "@/lib/api";
import { formatQuotaPrecise, formatRawQuota } from "@/lib/format";
import { QuotaAmount } from "./quota-amount";
import { handleServerError } from "@/lib/handle-server-error";
import { requireServerSuccess } from "@/lib/server-error-message";

import { downloadInvoiceCsv } from "../download";
import { profitRecordsParams } from "../lib/profit-records";
import type {
  ProfitRecord,
  ProfitRecordsPage as ProfitRecordsData,
  RevenueScope,
} from "../revenue-types";

const PAGE_SIZE = 20;
function recordsUrl(scope: RevenueScope): string {
  return scope === "admin"
    ? "/api/admin/revenue/records"
    : "/api/reseller/revenue/records";
}

export function ProfitRecordsPage(props: {
  scope: RevenueScope;
  tabSwitch?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  const admin = props.scope === "admin";
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [resellerId, setResellerId] = useState("");
  const [customerId, setCustomerId] = useState("");
  const [page, setPage] = useState(1);
  const [exporting, setExporting] = useState(false);
  const filters = profitRecordsParams({ from, to, resellerId, customerId });

  const records = useQuery({
    queryKey: ["profit-records", props.scope, filters, page],
    // Real-time: rows appear as calls are billed.
    refetchInterval: 15000,
    queryFn: async () => {
      const res = await api.get(recordsUrl(props.scope), {
        params: { ...filters, p: page, page_size: PAGE_SIZE },
      });
      return requireServerSuccess(res.data).data as ProfitRecordsData;
    },
  });

  const exportRecords = async (format: "pdf" | "csv") => {
    setExporting(true);
    try {
      await downloadInvoiceCsv(
        recordsUrl(props.scope),
        `profit-records-${props.scope}.${format}`,
        { ...filters, format },
      );
    } catch (error) {
      handleServerError(error, t("Download failed"));
    } finally {
      setExporting(false);
    }
  };

  const data = records.data;
  const totalPages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1;
  const ts = (v: number) => new Date(v * 1000).toLocaleString(intlLocale);
  const typeLabel = (r: ProfitRecord) => {
    const kind = r.kind === "refund" ? t("Refund") : t("Charge");
    let payer = t("Direct user");
    if (r.payer_type === "customer") payer = t("Customer");
    if (r.payer_type === "reseller_self") payer = t("Reseller self-use");
    return `${kind} · ${payer}`;
  };
  const resetPage =
    (set: (v: string) => void) => (e: { target: { value: string } }) => {
      set(e.target.value);
      setPage(1);
    };

  let content: ReactNode;
  if (records.isLoading) {
    content = <p className="text-sm">{t("Loading...")}</p>;
  } else if (records.isError || !data) {
    content = <p className="text-sm">{t("Failed to load")}</p>;
  } else {
    const tot = data.totals;
    content = (
      <div className="space-y-3" data-testid="profit-records">
        <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-sm">
          <span>
            {t("Records")}: {tot.rows}
          </span>
          <span>
            {t("Customer price")}: {formatQuotaPrecise(tot.customer_price)} (
            {formatRawQuota(tot.customer_price)} quota)
          </span>
          <span>
            {admin ? t("Reseller cost") : t("Your cost")}:{" "}
            {formatQuotaPrecise(tot.reseller_cost)} (
            {formatRawQuota(tot.reseller_cost)} quota)
          </span>
          <span>
            {admin ? t("Reseller profit") : t("Your profit")}:{" "}
            {formatQuotaPrecise(tot.reseller_profit)} (
            {formatRawQuota(tot.reseller_profit)} quota)
          </span>
          {admin ? (
            <span>
              {t("Admin profit")}: {formatQuotaPrecise(tot.admin_profit ?? 0)} (
              {formatRawQuota(tot.admin_profit ?? 0)} quota)
            </span>
          ) : null}
          {admin && data.admin_accumulator ? (
            <span>
              {t("Admin profit credited")}:{" "}
              {formatQuotaPrecise(data.admin_accumulator.credited_quota)} (
              {formatRawQuota(data.admin_accumulator.credited_quota)} quota) ·{" "}
              {t("Carried remainder")}:{" "}
              {(data.admin_accumulator.remainder_micro / 1e6).toFixed(6)} quota
            </span>
          ) : null}
        </div>
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Time")}</TableHead>
                <TableHead>{admin ? t("User") : t("Customer")}</TableHead>
                {admin ? <TableHead>{t("Reseller")}</TableHead> : null}
                <TableHead>{t("Model")}</TableHead>
                <TableHead>{t("Type")}</TableHead>
                <TableHead className="text-right">
                  {t("Customer price")}
                </TableHead>
                <TableHead className="text-right">
                  {admin ? t("Reseller cost") : t("Your cost")}
                </TableHead>
                <TableHead className="text-right">
                  {admin ? t("Reseller profit") : t("Your profit")}
                </TableHead>
                {admin ? (
                  <TableHead className="text-right">
                    {t("Admin profit")}
                  </TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={admin ? 9 : 7}
                    className="text-muted-foreground text-center"
                  >
                    {t("No records")}
                  </TableCell>
                </TableRow>
              ) : (
                data.items.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className="whitespace-nowrap">
                      {ts(r.created_at)}
                    </TableCell>
                    <TableCell>
                      {r.username || "—"} (#{r.user_id})
                    </TableCell>
                    {admin ? (
                      <TableCell>
                        {r.reseller_id
                          ? `${r.reseller_username || "—"} (#${r.reseller_id})`
                          : "—"}
                      </TableCell>
                    ) : null}
                    <TableCell>{r.model_name}</TableCell>
                    <TableCell>{typeLabel(r)}</TableCell>
                    <TableCell className="text-right">
                      <QuotaAmount quota={r.customer_price} showRaw />
                    </TableCell>
                    <TableCell className="text-right">
                      <QuotaAmount quota={r.reseller_cost} showRaw />
                    </TableCell>
                    <TableCell className="text-right">
                      <QuotaAmount quota={r.reseller_profit} showRaw />
                    </TableCell>
                    {admin ? (
                      <TableCell className="text-right">
                        <QuotaAmount quota={r.admin_profit ?? 0} showRaw />
                      </TableCell>
                    ) : null}
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        <div className="flex items-center justify-end gap-2 text-sm">
          <Button
            size="sm"
            variant="outline"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
          >
            {t("Previous")}
          </Button>
          <span>
            {page} / {totalPages}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("Next")}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t("Invoices")}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {props.tabSwitch}
        <Input
          type="date"
          className="h-8 w-36"
          aria-label={t("From")}
          value={from}
          onChange={resetPage(setFrom)}
        />
        <Input
          type="date"
          className="h-8 w-36"
          aria-label={t("To")}
          value={to}
          onChange={resetPage(setTo)}
        />
        {admin ? (
          <Input
            className="h-8 w-28"
            placeholder={t("Reseller ID")}
            aria-label={t("Reseller ID")}
            value={resellerId}
            onChange={resetPage(setResellerId)}
          />
        ) : null}
        <Input
          className="h-8 w-28"
          placeholder={t("Customer ID")}
          aria-label={t("Customer ID")}
          value={customerId}
          onChange={resetPage(setCustomerId)}
        />
        <Button size="sm" variant="outline" onClick={() => records.refetch()}>
          {t("Refresh")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={exporting}
          onClick={() => exportRecords("pdf")}
        >
          {t("Export")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={exporting}
          onClick={() => exportRecords("csv")}
        >
          {t("Export CSV")}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
