// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.app;

import com.openexchange.oms.api.auth.AuthenticationException;

/** Validates a Tyche session token and binds it to the supplied NECO address. */
@FunctionalInterface
public interface TycheSessionValidator {
    TycheSession validate(String sessionToken, String necoAddress) throws AuthenticationException;
}
