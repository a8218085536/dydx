// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.auth;

/** Default policy for admin, read, trade, and balance-funding actions. */
public final class RoleBasedAuthorizer implements Authorizer {

    @Override
    public boolean allow(Principal principal, String action, String resource) {
        if (principal == null) return false;
        boolean marketMaker = principal.hasRole(Principal.ROLE_MARKET_MAKER);
        if (ACTION_ADMIN.equals(action)) {
            return !marketMaker && principal.hasRole(Principal.ROLE_ADMIN);
        }

        final long requestedUserId;
        try {
            requestedUserId = Long.parseLong(resource);
        } catch (NumberFormatException e) {
            return false;
        }

        boolean ownUser = principal.userId() == requestedUserId;
        boolean anyUser = principal.hasRole(Principal.ROLE_ANY_USER);
        return switch (action) {
            case ACTION_ACT_AS_USER, ACTION_READ_AS_USER, ACTION_TRADE_AS_USER ->
                    ownUser || (!marketMaker && anyUser);
            case ACTION_FUND_AS_USER -> !marketMaker && (ownUser
                    || (anyUser && principal.hasRole(Principal.ROLE_FUNDING_ADMIN)));
            default -> false;
        };
    }
}
