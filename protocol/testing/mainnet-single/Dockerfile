FROM dydxprotocol-base
ENV GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct
ENV GOSUMDB=sum.golang.google.cn

RUN apk add --no-cache bash bc dasel jq
RUN go install cosmossdk.io/tools/cosmovisor/cmd/cosmovisor@v1.5.0

COPY ./testing/genesis.sh /dydxprotocol/genesis.sh
COPY ./testing/delaymsg_config /dydxprotocol/delaymsg_config
COPY ./daemons/pricefeed/client/constants/testdata /dydxprotocol/exchange_config
COPY ./testing/mainnet-single/entrypoint.sh /dydxprotocol/mainnet-single-entrypoint.sh
RUN chmod 755 /dydxprotocol/mainnet-single-entrypoint.sh

ENV HOME /dydxprotocol
ENV DAEMON_NAME dydxprotocold
ENV DAEMON_HOME /dydxprotocol/chain/.validator
WORKDIR $HOME

ENTRYPOINT ["/dydxprotocol/mainnet-single-entrypoint.sh"]
