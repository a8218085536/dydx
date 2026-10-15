// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.auth;

/**
 * SPI: decide whether a principal may perform an action on a resource.
 * Default policy is {@link RoleBasedAuthorizer}; integrators can substitute
 * their own (e.g. per-market permissions, read-only keys).
 */
public interface Authorizer {

    /** Admin endpoints (risk config, circuit breakers). Resource = request URI/method. */
    String ACTION_ADMIN = "admin";

    /** Acting on behalf of a user (orders, balances, deposit/withdraw). Resource = userId. */
    String ACTION_ACT_AS_USER = "act-as-user";

    /** Read account, balance, order, execution, or position data for a user. */
    String ACTION_READ_AS_USER = "read-as-user";

    /** Create, amend, or cancel orders for a user. */
    String ACTION_TRADE_AS_USER = "trade-as-user";

    /** Credit or debit a user's internal Assets Engine balance. */
    String ACTION_FUND_AS_USER = "fund-as-user";

    boolean allow(Principal principal, String action, String resource);
}
