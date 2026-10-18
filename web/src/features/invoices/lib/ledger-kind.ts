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
import type { TFunction } from "i18next";

/** Translated label for a balance-ledger kind; unknown kinds pass through. */
export function ledgerKindLabel(kind: string, t: TFunction): string {
  switch (kind) {
    case "admin_credit":
      return t("Admin credit");
    case "admin_debit":
      return t("Admin debit");
    case "customer_credit":
      return t("Customer credit");
    case "customer_debit":
      return t("Customer debit");
    case "redemption_issue":
      return t("Redemption code issued");
    case "redemption_refund":
      return t("Redemption code refund");
    case "redemption_redeem":
      return t("Redemption code redeemed");
    case "reseller_profit_credit":
      return t("Reseller profit credit");
    case "admin_profit_credit":
      return t("Admin profit credit");
    case "direct_credit_cost":
      return t("Direct credit cost");
    case "direct_credit_reseller_profit":
      return t("Direct credit reseller profit");
    case "direct_credit_admin_profit":
      return t("Direct credit admin profit");
    default:
      return kind;
  }
}
