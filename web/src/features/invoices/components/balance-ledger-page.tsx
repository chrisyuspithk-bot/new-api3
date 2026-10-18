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
import { QuotaAmount } from "./quota-amount";
import { handleServerError } from "@/lib/handle-server-error";

import {
  downloadInvoiceCsv,
  ledgerExportFilename,
  type LedgerExportFormat,
} from "../download";
import { ledgerKindLabel } from "../lib/ledger-kind";
import type { LedgerPage, LedgerQuery, ResellerLedgerKind } from "../types";

const LEDGER_PAGE_SIZE = 20;
const PERIOD_RE = /^\d{4}-(0[1-9]|1[0-2])$/;

type BalanceLedgerPageProps = {
  queryKey: string;
  loadRows: (query: LedgerQuery) => Promise<LedgerPage>;
  /** Role-scoped CSV endpoint; the server adds monthly credit, usage and fee totals. */
  exportUrl: string;
  kinds: readonly ResellerLedgerKind[];
  showReseller?: boolean;
  showCustomer?: boolean;
  /** When false, hide Operator/Remark (customer-facing). Default true. */
  showInternal?: boolean;
  /** Rendered first in the action bar (e.g. the Invoices tab switch). */
  extraActions?: ReactNode;
};

function usd(n: number) {
  return `$${n.toFixed(6)}`;
}

function currentMonthPeriod(): string {
  // Server periods use Asia/Hong_Kong; default the picker the same way.
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Hong_Kong",
    year: "numeric",
    month: "2-digit",
  }).format(new Date());
}

export function BalanceLedgerPage(props: BalanceLedgerPageProps) {
  const { t, i18n } = useTranslation();
  const [period, setPeriod] = useState(currentMonthPeriod);
  const [kind, setKind] = useState("");
  const [ledgerPage, setLedgerPage] = useState(1);
  const [exporting, setExporting] = useState(false);
  const periodValid = PERIOD_RE.test(period);
  const showReseller = props.showReseller === true;
  const showCustomer = props.showCustomer === true;
  const showInternal = props.showInternal !== false;
  const columnCount =
    4 +
    (showReseller ? 1 : 0) +
    (showCustomer ? 1 : 0) +
    (showInternal ? 2 : 0);
  const intlLocale = toIntlLocale(i18n.resolvedLanguage || i18n.language);

  const ledger = useQuery({
    queryKey: [props.queryKey, period, kind, ledgerPage],
    enabled: periodValid,
    queryFn: () =>
      props.loadRows({
        period,
        kind,
        page: ledgerPage,
        pageSize: LEDGER_PAGE_SIZE,
      }),
  });

  const exportLedger = async (format: LedgerExportFormat) => {
    setExporting(true);
    try {
      // The server answers a PDF statement by default, in the user's language.
      const params: Record<string, string> = { period };
      if (kind) params.kind = kind;
      if (format === "csv") params.format = "csv";
      await downloadInvoiceCsv(
        props.exportUrl,
        ledgerExportFilename(period, kind, format),
        params,
      );
    } catch (error) {
      handleServerError(error, t("Download failed"));
    } finally {
      setExporting(false);
    }
  };

  const rows = ledger.data?.items ?? [];
  const total = ledger.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / LEDGER_PAGE_SIZE));

  let tableBody: ReactNode;
  if (ledger.isLoading) {
    tableBody = (
      <TableRow>
        <TableCell colSpan={columnCount}>{t("Loading...")}</TableCell>
      </TableRow>
    );
  } else if (ledger.isError) {
    tableBody = (
      <TableRow>
        <TableCell colSpan={columnCount}>{t("Failed to load")}</TableCell>
      </TableRow>
    );
  } else if (rows.length === 0) {
    tableBody = (
      <TableRow>
        <TableCell colSpan={columnCount}>{t("No ledger rows")}</TableCell>
      </TableRow>
    );
  } else {
    tableBody = rows.map((row) => (
      <TableRow key={row.id}>
        <TableCell>
          {row.created_at
            ? new Date(row.created_at * 1000).toLocaleString(intlLocale)
            : "—"}
        </TableCell>
        {showReseller ? (
          <TableCell>
            {row.reseller_username || `#${row.reseller_id}`}
          </TableCell>
        ) : null}
        <TableCell>{ledgerKindLabel(row.kind, t)}</TableCell>
        <TableCell>
          <QuotaAmount quota={row.quota_delta} showRaw />
        </TableCell>
        <TableCell>
          {row.money_usd != null ? usd(Number(row.money_usd)) : "—"}
        </TableCell>
        {showCustomer ? (
          <TableCell>
            {row.customer_id
              ? row.customer_username || `#${row.customer_id}`
              : "—"}
          </TableCell>
        ) : null}
        {showInternal ? (
          <>
            <TableCell>
              {row.operator_id
                ? row.operator_username || `#${row.operator_id}`
                : "—"}
            </TableCell>
            <TableCell>{row.remark || "—"}</TableCell>
          </>
        ) : null}
      </TableRow>
    ));
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t("Invoices")}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {props.extraActions}
        <Input
          className="h-8 w-28"
          placeholder={t("YYYY-MM")}
          aria-label={t("Month")}
          value={period}
          onChange={(e) => {
            setPeriod(e.target.value);
            setLedgerPage(1);
          }}
        />
        <select
          className="border-input bg-background h-8 rounded-lg border px-2 text-sm"
          aria-label={t("Kind")}
          value={kind}
          onChange={(e) => {
            setKind(e.target.value);
            setLedgerPage(1);
          }}
        >
          <option value="">{t("All kinds")}</option>
          {props.kinds.map((value) => (
            <option key={value} value={value}>
              {ledgerKindLabel(value, t)}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          variant="outline"
          disabled={!periodValid}
          onClick={() => ledger.refetch()}
        >
          {t("Refresh")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={!periodValid || exporting}
          onClick={() => exportLedger("pdf")}
        >
          {t("Export")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={!periodValid || exporting}
          onClick={() => exportLedger("csv")}
        >
          {t("Export CSV")}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {!periodValid ? (
          <p className="text-muted-foreground text-sm">
            {t("Enter a period as YYYY-MM to load the balance ledger")}
          </p>
        ) : (
          <div className="space-y-3">
            <div className="rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("Time")}</TableHead>
                    {showReseller ? (
                      <TableHead>{t("Reseller")}</TableHead>
                    ) : null}
                    <TableHead>{t("Kind")}</TableHead>
                    <TableHead>{t("Delta")}</TableHead>
                    <TableHead>{t("USD")}</TableHead>
                    {showCustomer ? (
                      <TableHead>{t("Customer")}</TableHead>
                    ) : null}
                    {showInternal ? (
                      <>
                        <TableHead>{t("Operator")}</TableHead>
                        <TableHead>{t("Remark")}</TableHead>
                      </>
                    ) : null}
                  </TableRow>
                </TableHeader>
                <TableBody>{tableBody}</TableBody>
              </Table>
            </div>
            <div className="text-muted-foreground flex items-center justify-between text-sm">
              <span>
                {t("Total")}: {total}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={ledgerPage <= 1}
                  onClick={() => setLedgerPage((p) => p - 1)}
                >
                  {t("Previous")}
                </Button>
                <span>
                  {ledgerPage} / {totalPages}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={ledgerPage >= totalPages}
                  onClick={() => setLedgerPage((p) => p + 1)}
                >
                  {t("Next")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
