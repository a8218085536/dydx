// SPDX-License-Identifier: Apache-2.0
package com.openexchange.oms.core;

import com.openexchange.oms.common.domain.ExecutionReport;
import com.openexchange.oms.common.enums.OrderSide;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OmsCoreEngineTest {

    @Test
    void newlyAppliedTradeEmitsTakerAndMakerReportsWithoutPersistenceHandler() {
        OmsCoreEngine engine = new OmsCoreEngine(
                new OrderLifecycleManager(), new SyntheticOrderEngine());
        engine.setSettlementHandler((tradeId, buyerUserId, sellerUserId, marketId,
                                     price, quantity, buyerOmsOrderId, sellerOmsOrderId) -> true);
        List<ExecutionReport> reports = new ArrayList<>();
        engine.setExecutionListener(reports::add);

        engine.onTradeExecution(
                1, 9001L, 7001L, 7002L,
                101L, 202L, 123_45000000L, 2_00000000L,
                true, 5001L, 5002L, 77L);

        assertEquals(2, reports.size());
        ExecutionReport taker = reports.get(0);
        assertEquals(9001L, taker.getTradeId());
        assertEquals(5001L, taker.getOmsOrderId());
        assertEquals(7001L, taker.getClusterOrderId());
        assertEquals(101L, taker.getUserId());
        assertEquals(1, taker.getMarketId());
        assertEquals(OrderSide.BUY, taker.getSide());
        assertEquals(123_45000000L, taker.getPrice());
        assertEquals(2_00000000L, taker.getQuantity());
        assertFalse(taker.isMaker());

        ExecutionReport maker = reports.get(1);
        assertEquals(5002L, maker.getOmsOrderId());
        assertEquals(7002L, maker.getClusterOrderId());
        assertEquals(202L, maker.getUserId());
        assertEquals(OrderSide.SELL, maker.getSide());
        assertTrue(maker.isMaker());
    }

    @Test
    void duplicateTradeDoesNotEmitExecutionReports() {
        OmsCoreEngine engine = new OmsCoreEngine(
                new OrderLifecycleManager(), new SyntheticOrderEngine());
        engine.setSettlementHandler((tradeId, buyerUserId, sellerUserId, marketId,
                                     price, quantity, buyerOmsOrderId, sellerOmsOrderId) -> false);
        List<ExecutionReport> reports = new ArrayList<>();
        engine.setExecutionListener(reports::add);

        engine.onTradeExecution(
                1, 9001L, 7001L, 7002L,
                101L, 202L, 123_45000000L, 2_00000000L,
                true, 5001L, 5002L, 77L);

        assertTrue(reports.isEmpty());
    }
}
