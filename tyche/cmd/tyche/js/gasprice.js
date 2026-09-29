var GWEI = 1000000000;

var Polygon = {}
Polygon.id          = 7256
Polygon.maxGasPrice = 1000 * GWEI
Polygon.multiplier  = 1.1
Polygon.bumpingGas = function(gasPrice)  {
    var bumpedGas = Math.ceil(gasPrice * this.multiplier);
    if (bumpedGas > this.maxGasPrice) {
        return 0;
    }
    return bumpedGas;
}
Polygon.suggestGasPrice = function(gasPrice) {
    var gwei = gasPrice / GWEI;
    if (gwei <= 40) {
        //return 30 * GWEI
        return Math.ceil(gasPrice * 1.05)
    } else if (gwei <= 80) {
        //return 65 *GWEI
        return Math.ceil(gasPrice * 1.03)
    } else if (gwei <= 120) {
        //return 100 * gwei
        return Math.ceil(gasPrice * 1.01)
    } else if (gwei <= 500) {
        // 高峰时段， 建议价格的 1.1 倍
        return Math.ceil(gasPrice * 1.1)
    } else {
        // 繁忙时段， 建议价格的 1.2 倍
        return Math.ceil(gasPrice * 1.2)
    }
}

var Default = {}
Default.multiplier  = 1.1
Default.bumpingGas = function(gasPrice) {
    Math.ceil(gasPrice * this.multiplier);
}
Default.suggestGasPrice = function(gasPrice) {
    var gwei = gasPrice / GWEI;
    if (gwei <= 10) {
        return 6 * GWEI;
    } else if (gwei <= 25) {
        return 15 * GWEI;
    } else if (gwei <= 40) {
        return 30 * GWEI;
    } else if (gwei <= 100) {
        // 高峰时段， 建议价格的 1.1 倍
        return Math.ceil(gasPrice * 1.1)
    } else {
        // 繁忙时段， 建议价格的 1.2 倍
        return Math.ceil(gasPrice * 1.2)
    }
}

function suggestGasPrice(chainID, gasPrice) {
    switch (chainID) {
        case Polygon.id:
            return Polygon.suggestGasPrice(gasPrice)
        default:      
            return Default.suggestGasPrice(gasPrice)
    }
}

function bumpingGas(chainID, gasPrice) {
    switch (chainID) {
        case Polygon.id:
            return Polygon.bumpingGas(gasPrice)
        default:      
            return Default.bumpingGas(gasPrice)
    }
}