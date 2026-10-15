// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

/** Authenticated Tyche wallet-login identity. */
public record TycheSession(String tycheUserId, String necoAddress) {}
