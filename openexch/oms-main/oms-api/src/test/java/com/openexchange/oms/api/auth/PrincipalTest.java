// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.api.auth;

import org.junit.jupiter.api.Test;

import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

class PrincipalTest {

    @Test
    void ownUserOnlyWithoutAnyUserRole() {
        Principal p = new Principal("k1", 7, Set.of());
        assertTrue(p.canActAs(7));
        assertFalse(p.canActAs(8));
    }

    @Test
    void anyUserRoleActsAsEveryone() {
        Principal p = new Principal("svc", 1, Set.of(Principal.ROLE_ANY_USER));
        assertTrue(p.canActAs(1));
        assertTrue(p.canActAs(999_999));
    }

    @Test
    void resolveUserIdDefaultsToPrincipal() {
        Principal p = new Principal("k1", 7, Set.of());
        assertEquals(7, p.resolveUserId(0));
        assertEquals(3, p.resolveUserId(3));
    }

    @Test
    void roleBasedAuthorizer() {
        Authorizer authz = new RoleBasedAuthorizer();
        Principal admin = new Principal("a", 1, Set.of(Principal.ROLE_ADMIN));
        Principal user = new Principal("u", 2, Set.of());

        assertTrue(authz.allow(admin, Authorizer.ACTION_ADMIN, "PUT /api/v1/admin/risk/config/1"));
        assertFalse(authz.allow(user, Authorizer.ACTION_ADMIN, "PUT /api/v1/admin/risk/config/1"));
        assertTrue(authz.allow(user, Authorizer.ACTION_ACT_AS_USER, "2"));
        assertFalse(authz.allow(user, Authorizer.ACTION_ACT_AS_USER, "3"));
        assertFalse(authz.allow(user, Authorizer.ACTION_ACT_AS_USER, "not-a-number"));
        assertFalse(authz.allow(null, Authorizer.ACTION_ACT_AS_USER, "2"));
    }

    @Test
    void marketMakerMayOnlyReadAndTradeAsItself() {
        Authorizer authz = new RoleBasedAuthorizer();
        Principal maker = new Principal("maker", 910001,
                Set.of(Principal.ROLE_MARKET_MAKER));

        assertTrue(authz.allow(maker, Authorizer.ACTION_READ_AS_USER, "910001"));
        assertTrue(authz.allow(maker, Authorizer.ACTION_TRADE_AS_USER, "910001"));
        assertFalse(authz.allow(maker, Authorizer.ACTION_FUND_AS_USER, "910001"));
        assertFalse(authz.allow(maker, Authorizer.ACTION_READ_AS_USER, "910002"));
        assertFalse(authz.allow(maker, Authorizer.ACTION_TRADE_AS_USER, "910002"));
        assertFalse(authz.allow(maker, Authorizer.ACTION_FUND_AS_USER, "910002"));
        assertFalse(authz.allow(maker, Authorizer.ACTION_ADMIN, "GET /api/v1/admin/risk/config"));
    }

    @Test
    void ordinaryUsersRetainSelfScopedReadTradeAndFunding() {
        Authorizer authz = new RoleBasedAuthorizer();
        Principal user = new Principal("wallet", 42, Set.of());

        assertTrue(authz.allow(user, Authorizer.ACTION_READ_AS_USER, "42"));
        assertTrue(authz.allow(user, Authorizer.ACTION_TRADE_AS_USER, "42"));
        assertTrue(authz.allow(user, Authorizer.ACTION_FUND_AS_USER, "42"));
    }

    @Test
    void crossUserFundingRequiresBothFundingRolesAndRejectsMarketMakers() {
        Authorizer authz = new RoleBasedAuthorizer();
        Principal anyUserOnly = new Principal("any", 1, Set.of(Principal.ROLE_ANY_USER));
        Principal fundingOnly = new Principal("fund", 1, Set.of(Principal.ROLE_FUNDING_ADMIN));
        Principal fundingAdmin = new Principal("operator", 1,
                Set.of(Principal.ROLE_ANY_USER, Principal.ROLE_FUNDING_ADMIN));
        Principal makerWithEveryRole = new Principal("maker", 910001,
                Set.of(Principal.ROLE_MARKET_MAKER, Principal.ROLE_ANY_USER,
                        Principal.ROLE_FUNDING_ADMIN, Principal.ROLE_ADMIN));

        assertFalse(authz.allow(anyUserOnly, Authorizer.ACTION_FUND_AS_USER, "910001"));
        assertFalse(authz.allow(fundingOnly, Authorizer.ACTION_FUND_AS_USER, "910001"));
        assertTrue(authz.allow(fundingAdmin, Authorizer.ACTION_FUND_AS_USER, "910001"));
        assertFalse(authz.allow(makerWithEveryRole, Authorizer.ACTION_FUND_AS_USER, "910001"));
        assertFalse(authz.allow(makerWithEveryRole, Authorizer.ACTION_READ_AS_USER, "910002"));
        assertFalse(authz.allow(makerWithEveryRole, Authorizer.ACTION_ADMIN, "anything"));
    }
}
