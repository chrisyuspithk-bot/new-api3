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
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { formatQuotaPrecise, formatRawQuota } from "@/lib/format";

import { QuotaAmount } from "../components/quota-amount";

describe("invoice amount precision", () => {
  it("shows tiny calls with 6 decimals (500,000 quota = $1)", () => {
    expect(formatQuotaPrecise(62)).toBe("$0.000124");
    expect(formatQuotaPrecise(1)).toBe("$0.000002");
    expect(formatQuotaPrecise(-10)).toBe("-$0.00002");
  });
  it("row amounts add up to the displayed total", () => {
    const rows = [62, 52, 10];
    const total = rows.reduce((a, b) => a + b, 0);
    expect(formatQuotaPrecise(total)).toBe("$0.000248");
  });
  it("raw quota keeps integers and micro fractions", () => {
    expect(formatRawQuota(2492856)).toBe("2,492,856");
    expect(formatRawQuota(8.944444)).toBe("8.944444");
  });
  it("QuotaAmount renders the raw quota value and tooltip", () => {
    render(<QuotaAmount quota={62} showRaw />);
    expect(screen.getByText("62 Quota")).toBeTruthy();
    expect(screen.getByTitle("62 Quota")).toBeTruthy();
  });
});
