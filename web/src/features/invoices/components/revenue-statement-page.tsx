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
import { Badge } from "@/components/ui/badge";
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
import { QuotaAmount } from "./quota-amount";
import { handleServerError } from "@/lib/handle-server-error";
import { requireServerSuccess } from "@/lib/server-error-message";

import { downloadInvoiceCsv } from "../download";
import type {
  RevenueAgg,
  RevenueScope,
  RevenueStatement,
} from "../revenue-types";

const PERIOD_RE = /^\d{4}-(0[1-9]|1[0-2])$/;

function currentMonthPeriod(): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Hong_Kong",
    year: "numeric",
    month: "2-digit",
  }).format(new Date());
}

function statementUrl(scope: RevenueScope): string {
  return scope === "admin"
    ? "/api/admin/revenue/statement"
    : "/api/reseller/revenue/statement";
}

/** micro-quota (1e-6) to quota */
function micro(v?: number): number {
  return (v ?? 0) / 1_000_000;
}

export function RevenueStatementPage(props: {
  scope: RevenueScope;
  tabSwitch?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  const [period, setPeriod] = useState(currentMonthPeriod);
  const [exporting, setExporting] = useState(false);
  const periodValid = PERIOD_RE.test(period);
  const admin = props.scope === "admin";
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language);

  const statement = useQuery({
    queryKey: ["revenue-statement", props.scope, period],
    enabled: periodValid,
    queryFn: async () => {
      const res = await api.get(statementUrl(props.scope), {
        params: { period },
      });
      return requireServerSuccess(res.data).data as RevenueStatement;
    },
  });

  const exportStatement = async (format: "pdf" | "csv") => {
    setExporting(true);
    try {
      await downloadInvoiceCsv(
        statementUrl(props.scope),
        `revenue-${period}-${props.scope}.${format}`,
        { period, format },
      );
    } catch (error) {
      handleServerError(error, t("Download failed"));
    } finally {
      setExporting(false);
    }
  };

  const st = statement.data;
  const ts = (v?: number) =>
    v ? new Date(v * 1000).toLocaleString(intlLocale) : "—";

  const lineTable = (title: string, lines: RevenueAgg[] | undefined) => (
    <div className="space-y-2">
      <h3 className="text-sm font-semibold">{title}</h3>
      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("Name")}</TableHead>
              <TableHead className="text-right">{t("Calls")}</TableHead>
              <TableHead className="text-right">{t("Refunds")}</TableHead>
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
                <>
                  <TableHead className="text-right">
                    {t("Base price")}
                  </TableHead>
                  <TableHead className="text-right">
                    {t("Admin margin")}
                  </TableHead>
                </>
              ) : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {!lines || lines.length === 0 ? (
              <TableRow>
                <TableCell colSpan={admin ? 8 : 6}>
                  {t("No revenue in this period")}
                </TableCell>
              </TableRow>
            ) : (
              lines.map((a) => (
                <TableRow key={a.key}>
                  <TableCell>{a.name || `#${a.id}`}</TableCell>
                  <TableCell className="text-right">{a.requests}</TableCell>
                  <TableCell className="text-right">{a.refunds}</TableCell>
                  <TableCell className="text-right">
                    <QuotaAmount quota={a.customer_price} />
                  </TableCell>
                  <TableCell className="text-right">
                    <QuotaAmount quota={a.reseller_cost} />
                  </TableCell>
                  <TableCell className="text-right">
                    <QuotaAmount quota={a.reseller_profit} />
                  </TableCell>
                  {admin ? (
                    <>
                      <TableCell className="text-right">
                        <QuotaAmount quota={micro(a.base_quota_micro)} />
                      </TableCell>
                      <TableCell className="text-right">
                        <QuotaAmount quota={micro(a.admin_profit_micro)} />
                      </TableCell>
                    </>
                  ) : null}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );

  let summaryRows: Array<[string, number | null, number]> = [];
  if (st && admin) {
    summaryRows = [
      [
        t("Total billed to payers"),
        st.totals.requests,
        st.totals.customer_price,
      ],
      [
        t("Reseller customer sales"),
        st.customer_total.requests,
        st.customer_total.customer_price,
      ],
      [
        t("Reseller cost (paid to admin)"),
        null,
        st.customer_total.reseller_cost,
      ],
      [t("Reseller profit"), null, st.customer_total.reseller_profit],
      [
        t("Reseller self-use"),
        st.self_total.requests,
        st.self_total.customer_price,
      ],
      [
        t("Direct user charges"),
        st.direct_total.requests,
        st.direct_total.customer_price,
      ],
      [t("Admin revenue"), null, st.totals.admin_revenue],
      [t("Base price total"), null, micro(st.totals.base_quota_micro)],
      [t("Admin margin over base"), null, micro(st.totals.admin_profit_micro)],
    ];
  } else if (st) {
    summaryRows = [
      [
        t("Sales to your customers"),
        st.customer_total.requests,
        st.customer_total.customer_price,
      ],
      [
        t("Your cost for customer usage"),
        null,
        st.customer_total.reseller_cost,
      ],
      [t("Your profit"), null, st.customer_total.reseller_profit],
      [
        t("Your own usage"),
        st.self_total.requests,
        st.self_total.reseller_cost,
      ],
      [
        t("Total paid to admin"),
        null,
        st.customer_total.reseller_cost + st.self_total.reseller_cost,
      ],
    ];
  }

  const checkBadge = (ok: boolean | null) => {
    if (ok === null) return <Badge variant="outline">{t("Not checked")}</Badge>;
    if (ok) return <Badge variant="secondary">{t("OK")}</Badge>;
    return <Badge variant="destructive">{t("MISMATCH")}</Badge>;
  };

  const rec = st?.reconciliation;
  const checkRows: Array<[string, string, boolean | null]> =
    st && rec
      ? [
          ...(admin
            ? ([
                [
                  t("Charge = base + admin margin + reseller profit"),
                  String(micro(rec.identity_diff_micro)),
                  rec.identity_ok,
                ],
              ] as Array<[string, string, boolean | null]>)
            : []),
          [t("Sections add up to total"), "", rec.sections_ok],
          [
            t("Revenue vs usage logs (charge)"),
            rec.log_compared ? String(rec.log_charge_diff) : "—",
            rec.log_compared ? rec.log_charge_diff === 0 : null,
          ],
          [
            t("Revenue vs usage logs (reseller cost)"),
            rec.log_compared ? String(rec.log_reseller_cost_diff) : "—",
            rec.log_compared ? rec.log_reseller_cost_diff === 0 : null,
          ],
          [
            t("Records with invalid factors"),
            String(rec.flagged_rows),
            rec.flagged_rows === 0,
          ],
          [
            t("Records added after freeze"),
            String(st.late_rows),
            st.late_rows === 0 && !st.checksum_mismatch,
          ],
          [
            t("Records waiting for retry"),
            String(rec.pending_retry),
            rec.pending_retry === 0,
          ],
        ]
      : [];

  let content: ReactNode;
  if (!periodValid) {
    content = (
      <p className="text-muted-foreground text-sm">
        {t("Enter a period as YYYY-MM to load the revenue statement")}
      </p>
    );
  } else if (statement.isLoading) {
    content = <p className="text-sm">{t("Loading...")}</p>;
  } else if (statement.isError || !st) {
    content = <p className="text-sm">{t("Failed to load")}</p>;
  } else {
    content = (
      <div className="space-y-5" data-testid="revenue-statement">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant={st.frozen ? "secondary" : "outline"}>
            {st.frozen
              ? t("Closed month — frozen")
              : t("Open month — live figures")}
          </Badge>
          {st.late_rows !== 0 || st.checksum_mismatch ? (
            <Badge variant="destructive">
              {t("Records added after freeze")}: {st.late_rows}
            </Badge>
          ) : null}
          <span className="text-muted-foreground">
            {admin ? "" : `${st.reseller_name ?? ""} · `}
            {t("Revenue records")}: {st.row_count} · {st.timezone}
            {st.frozen ? ` · ${t("Frozen at")} ${ts(st.frozen_at)}` : ""}
          </span>
        </div>
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("Item")}</TableHead>
                <TableHead className="text-right">{t("Calls")}</TableHead>
                <TableHead className="text-right">{t("Amount")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {summaryRows.map(([label, calls, amount]) => (
                <TableRow key={label}>
                  <TableCell>{label}</TableCell>
                  <TableCell className="text-right">{calls ?? ""}</TableCell>
                  <TableCell className="text-right font-medium">
                    <QuotaAmount quota={amount} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        {admin ? (
          <>
            {lineTable(t("Customer sales by reseller"), st.by_reseller)}
            {lineTable(t("Reseller self-use"), st.reseller_self)}
            {lineTable(t("Direct users"), st.direct)}
          </>
        ) : (
          lineTable(t("Sales by customer"), st.by_customer)
        )}
        <div className="space-y-2">
          <h3 className="text-sm font-semibold">{t("Reconciliation")}</h3>
          <div className="rounded-lg border">
            <Table>
              <TableBody>
                {checkRows.map(([label, value, ok]) => (
                  <TableRow key={label}>
                    <TableCell>{label}</TableCell>
                    <TableCell className="text-right">{value}</TableCell>
                    <TableCell className="text-right">
                      {checkBadge(ok)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <p className="text-muted-foreground font-mono text-xs break-all">
            SHA-256 {st.checksum}
          </p>
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
          className="h-8 w-28"
          placeholder={t("YYYY-MM")}
          aria-label={t("Month")}
          value={period}
          onChange={(e) => setPeriod(e.target.value)}
        />
        <Button
          size="sm"
          variant="outline"
          disabled={!periodValid}
          onClick={() => statement.refetch()}
        >
          {t("Refresh")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={!periodValid || exporting}
          onClick={() => exportStatement("pdf")}
        >
          {t("Export")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={!periodValid || exporting}
          onClick={() => exportStatement("csv")}
        >
          {t("Export CSV")}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
