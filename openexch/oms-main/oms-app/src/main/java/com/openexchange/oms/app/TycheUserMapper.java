// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

/** Maps Tyche string user IDs to the numeric IDs used by OMS money and order paths. */
@FunctionalInterface
public interface TycheUserMapper {
    long findOrCreateOmsUserId(String tycheUserId, String necoAddress);
}
