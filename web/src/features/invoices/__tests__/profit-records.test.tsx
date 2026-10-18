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
import { describe, expect, it } from "vitest";

import { hktDayStart, profitRecordsParams } from "../lib/profit-records";

describe("profit records filters", () => {
  it("maps HKT calendar days to [start, end) unix seconds", () => {
    expect(hktDayStart("2026-10-18")).toBe(Date.UTC(2026, 9, 17, 16) / 1000);
    expect(hktDayStart("bad")).toBeUndefined();
    const p = profitRecordsParams({
      from: "2026-10-01",
      to: "2026-10-18",
      resellerId: "6",
      customerId: "15",
    });
    expect(p.start_timestamp).toBe(String(Date.UTC(2026, 8, 30, 16) / 1000));
    expect(p.end_timestamp).toBe(String(Date.UTC(2026, 9, 18, 16) / 1000));
    expect(p.reseller_id).toBe("6");
    expect(p.customer_id).toBe("15");
  });
  it("drops empty or non-numeric filters", () => {
    expect(
      profitRecordsParams({
        from: "",
        to: "",
        resellerId: "x",
        customerId: "",
      }),
    ).toEqual({});
  });
});
