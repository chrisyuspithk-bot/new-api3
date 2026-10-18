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
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/** HKT calendar day (YYYY-MM-DD) to a unix timestamp at 00:00 HKT. */
export function hktDayStart(day: string): number | undefined {
  if (!DATE_RE.test(day)) return undefined;
  const ms = Date.parse(`${day}T00:00:00+08:00`);
  return Number.isNaN(ms) ? undefined : Math.floor(ms / 1000);
}

/** Builds the query for the list and exports; `to` is inclusive (whole day). */
export function profitRecordsParams(f: {
  from: string;
  to: string;
  resellerId: string;
  customerId: string;
}): Record<string, string> {
  const p: Record<string, string> = {};
  const start = hktDayStart(f.from);
  const end = hktDayStart(f.to);
  if (start !== undefined) p.start_timestamp = String(start);
  if (end !== undefined) p.end_timestamp = String(end + 86400);
  if (/^\d+$/.test(f.resellerId)) p.reseller_id = f.resellerId;
  if (/^\d+$/.test(f.customerId)) p.customer_id = f.customerId;
  return p;
}
