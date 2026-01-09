# WebSocket Trading Stockbit - Dokumentasi Lengkap

## Overview

WebSocket trading Stockbit menggunakan Protocol Buffers (Protobuf) untuk komunikasi real-time pada endpoint `wss://wss-trading.stockbit.com/ws`. Sistem ini dibangun dengan **Scarlet WebSocket Framework** untuk Android dengan arsitektur multi-layer yang robust.

## Arsitektur Sistem

### 1. Endpoint Configuration
- **Production**: `wss://wss-trading.stockbit.com/ws`
- **Staging**: Disesuaikan berdasarkan environment
- Konfigurasi di `com/stockbit/android/di/C4070b.java` method `G()`
- Manageable via AppConfig di `com/stockbit/lib/appconfig/android/ui/AppConfigViewModel.java`

### 2. Technology Stack
- **WebSocket Client**: Scarlet Framework (Tinder/Scarlet)
- **Serialization**: Protocol Buffers (Protobuf)
- **Reactive Streams**: Kotlin Flow
- **Dependency Injection**: Koin
- **Lifecycle Management**: Custom lifecycle dengan Android Lifecycle

### 3. Package Structure
- Protobuf Definitions: `com.stockbit.protobuf.securities.transactional.datafeed.v1.datafeed`
- API Interface: `com.stockbit.remote.api.securities`
- DataSource: `com.stockbit.remote.datasource`
- Repository: `com.stockbit.repositories.websocket.trading`
- UseCase/Interactor: Layer business logic

## Layer Architecture

```
┌─────────────────────────────────────────┐
│            UI Layer (ViewModel)         │
├─────────────────────────────────────────┤
│        Repository Layer                 │
│  • WebSocketTradingRepositoryImpl       │
├─────────────────────────────────────────┤
│        DataSource Layer                 │
│  • WebSocketTradingDataSourceImpl       │
├─────────────────────────────────────────┤
│         API Layer (Scarlet)             │
│  • WebSocketTradingApi (Interface)      │
├─────────────────────────────────────────┤
│      Transport Layer (WebSocket)        │
│  • wss://wss-trading.stockbit.com/ws    │
└─────────────────────────────────────────┘
```


## Message Definitions

### 1. WebsocketRequest
Message untuk request subscribe/unsubscribe ke WebSocket trading.

```protobuf
message WebsocketRequest {
    string user_id = 1;          // ID pengguna
    WebsocketChannel channel = 2; // Channel yang di-subscribe
    string key = 3;              // Key autentikasi
    PingRequest ping = 4;        // Request ping (opsional)
}
```

**Field:**
- `user_id`: ID unik pengguna Stockbit
- `channel`: Konfigurasi channel yang ingin di-subscribe (lihat `WebsocketChannel`)
- `key`: Key autentikasi untuk verifikasi sesi
- `ping`: Request untuk ping/pong keep-alive

### 2. WebsocketChannel
Mendefinisikan channel-channel yang dapat di-subscribe untuk data trading.

```protobuf
message WebsocketChannel {
    repeated string watchlist = 1;           // Kode saham untuk watchlist
    repeated string order_book = 2;          // Kode saham untuk order book
    repeated string running_trade = 3;       // Kode saham untuk running trade
    bool is_hotlist = 4;                    // Flag untuk hotlist
    repeated string running_trade_batch = 5; // Kode saham untuk running trade batch
    repeated string liveprice = 6;           // Kode saham untuk live price
    repeated string iepiev = 7;              // Kode saham untuk IEP/IEV
    repeated string intraday = 8;            // Kode saham untuk data intraday
    repeated string best_bid_offer = 9;      // Kode saham untuk best bid offer
    repeated MarketMoverWebsocketRequest market_mover = 10; // Market mover
}
```

**Channel Types:**
- `watchlist`: Update harga untuk saham dalam watchlist
- `order_book`: Data order book (bids & asks)
- `running_trade`: Data transaksi real-time
- `running_trade_batch`: Batch running trade untuk efisiensi
- `liveprice`: Harga live
- `iepiev`: Data Indeks Efek Pasif/Indeks Efek Volatil
- `intraday`: Data intraday (grafik)
- `best_bid_offer`: Best bid dan offer
- `market_mover`: Saham dengan pergerakan signifikan

### 3. RunningTrade
Message untuk data transaksi real-time (running trade).

```protobuf
message RunningTrade {
    Timestamp websocket_time = 1;  // Waktu dari WebSocket server
    string stock = 2;              // Kode saham (e.g., "BBCA", "TLKM")
    double price = 3;              // Harga transaksi
    double volume = 4;             Volume transaksi
    TradeType action = 5;          // Jenis transaksi (BUY/SELL)
    bool is_global = 6;           // Flag untuk transaksi global
    Timestamp time = 7;            // Waktu transaksi
    Change change = 8;             // Perubahan harga
    int32 trade_number = 9;        // Nomor urut transaksi
    BoardType market_board = 10;   // Tipe papan perdagangan
}
```

**Enum TradeType:**
- `UNKNOWN_TRADE_TYPE`: Tidak diketahui
- `BUY`: Transaksi beli
- `SELL`: Transaksi jual
- `UNDISCLOSED`: Tidak diungkapkan

**Enum BoardType:**
- `REGULAR`: Papan reguler
- `NEGOTIATED`: Papan negoisasi
- `CASH`: Papan tunai
- `etc.` (ada banyak nilai lain)

### 4. Change
Message untuk perubahan harga.

```protobuf
message Change {
    double value = 1;     // Nilai perubahan
    double percent = 2;   // Persentase perubahan
}
```

### 5. Orderbook
Message untuk data order book.

```protobuf
message Orderbook {
    OrderBookHeader header = 1;    // Header order book
    OrderBookBody body = 2;        // Body order book (bids & asks)
}

message OrderBookHeader {
    string stock = 1;      // Kode saham
    Timestamp time = 2;    // Waktu update
}

message OrderBookBody {
    repeated Bid bids = 1;   // Daftar bid (penawaran beli)
    repeated Offer offers = 2; // Daftar offer (penawaran jual)
}

message Bid {
    double price = 1;      // Harga bid
    double volume = 2;     // Volume bid
    int32 order_count = 3; // Jumlah order
}

message Offer {
    double price = 1;      // Harga offer
    double volume = 2;     // Volume offer
    int32 order_count = 3; // Jumlah order
}
```

### 6. BestBidOffer
Message untuk best bid dan offer.

```protobuf
message BestBidOffer {
    string stock = 1;          // Kode saham
    double bid_price = 2;      // Harga bid terbaik
    double bid_volume = 3;     // Volume bid terbaik
    double offer_price = 4;    // Harga offer terbaik
    double offer_volume = 5;   // Volume offer terbaik
    Timestamp time = 6;        // Waktu update
}
```

### 7. RunningTradeBatch
Message batch untuk multiple running trade.

```protobuf
message RunningTradeBatch {
    repeated RunningTrade trades = 1;  // Daftar running trade
    Timestamp batch_time = 2;          // Waktu batch
}
```

### 8. PingRequest / PingResponse
Message untuk mekanisme ping/pong keep-alive.

```protobuf
message PingRequest {
    int64 timestamp = 1;  // Timestamp ping
}

message PingResponse {
    int64 timestamp = 1;  // Timestamp pong (biasanya sama dengan request)
}
```

### 9. Error
Message untuk error handling.

```protobuf
message Error {
    ErrorCode code = 1;    // Kode error
    string message = 2;    // Pesan error
}

enum ErrorCode {
    UNKNOWN_ERROR = 0;
    UNAUTHORIZED = 1;
    INVALID_REQUEST = 2;
    CHANNEL_LIMIT_EXCEEDED = 3;
    // ... kode error lainnya
}
```

## Data Flow WebSocket

### 1. Subscribe Request
Client mengirim `WebsocketRequest` ke server:

```java
WebsocketRequest request = WebsocketRequest.newBuilder()
    .setUserId("user123")
    .setKey("auth_key_xyz")
    .setChannel(WebsocketChannel.newBuilder()
        .addAllRunningTrade(Arrays.asList("BBCA", "TLKM", "BMRI"))
        .addAllOrderBook(Arrays.asList("BBCA"))
        .build())
    .build();
```

### 2. Server Response
Server akan mengirim data sesuai channel yang di-subscribe:

- **RunningTrade**: Stream `RunningTrade` message untuk setiap transaksi
- **Orderbook**: Stream `Orderbook` message saat order book update
- **BestBidOffer**: Stream `BestBidOffer` untuk best bid/offer update
- **RunningTradeBatch**: Batch `RunningTradeBatch` untuk efisiensi

### 3. Ping/Pong Keep-alive
Client dapat mengirim `PingRequest` secara berkala, server merespon dengan `PingResponse`.

## Implementasi Scarlet WebSocket API

### WebSocketTradingApi Interface
Interface utama untuk Scarlet WebSocket dengan annotation `@a` untuk streaming dan `@b` untuk send:

```java
public interface WebSocketTradingApi {
    @a  // Scarlet annotation untuk stream observasi
    @NotNull
    Flow<l.a> observeEvent();  // Observasi event koneksi WebSocket
    
    @a
    @NotNull
    Flow<Websocket$WebsocketResponse> observeResponse();  // Observasi response data
    
    @b  // Scarlet annotation untuk send message
    boolean sendPayload(@NotNull Websocket$WebsocketRequest request);  // Kirim payload
}
```

### WebSocketTradingDataSourceImpl
Implementasi data source yang menghubungkan API dengan repository:

```java
public final class WebSocketTradingDataSourceImpl implements m0 {
    private final WebSocketTradingApi f104500a;
    
    public Flow observeEvent() {
        return FlowKt.catch(this.f104500a.observeEvent(), 
            new WebSocketTradingDataSourceImpl$observeEvent$1(this, null));
    }
    
    public Flow observeResponse() {
        return FlowKt.catch(this.f104500a.observeResponse(),
            new WebSocketTradingDataSourceImpl$observeResponse$1(this, null));
    }
    
    public boolean sendPayload(Websocket$WebsocketRequest payload) {
        try {
            return this.f104500a.sendPayload(payload);
        } catch (Exception e) {
            // Error logging
            return false;
        }
    }
}
```

### WebSocketTradingRepositoryImpl
Repository layer dengan business logic dan coroutine management:

```java
public final class WebSocketTradingRepositoryImpl implements a {
    private final com.stockbit.features.model.a dispatchers;
    private final m0 webSocketTradingDataSource;
    private final r cacheTradingDataSource;
    private final CoroutineScope coroutineScope;
    private final Mutex mutex;
    private final MutableStateFlow statusFlow;
    private final Set<String> subscribedChannels;
    
    public void sendPayload(com.stockbit.domain.model.websocket.trading.a payload) {
        BuildersKt.launch$default(this.coroutineScope, null, null,
            new WebSocketTradingRepositoryImpl$sendPayload$1(this, payload, null), 3, null);
    }
    
    public StateFlow observeStatus() {
        return FlowKt.asStateFlow(this.statusFlow);
    }
    
    public Flow observeResponse() {
        return FlowKt.filterNotNull(FlowKt.asSharedFlow(responseFlow));
    }
}
```

### Lifecycle Management
Scarlet menggunakan custom lifecycle untuk mengatur koneksi WebSocket:

```java
// Di C4070b.java (DI Module)
public final com.tinder.scarlet.c B(
    com.tinder.scarlet.c applicationCreatedLifecycle,
    com.tinder.scarlet.c connectionAvailableLifecycle, 
    com.tinder.scarlet.c loggedInLifecycle) {
    
    return applicationCreatedLifecycle
        .a(connectionAvailableLifecycle)
        .a(loggedInLifecycle);
}

// Lifecycle components:
// 1. ApplicationCreatedLifecycle - App start
// 2. ConnectionAvailableLifecycle - Network available
// 3. LoggedInLifecycle - User authenticated
```

## Dependency Injection Setup

### Dagger/Koin Configuration
WebSocket trading di-inject melalui dependency injection:

```kotlin
// Di android/b.java
@Provides
fun provideWebSocketTradingApi(): WebSocketTradingApi {
    return Scarlet.Builder()
        .webSocketFactory(okHttpClient.newWebSocketFactory("wss://wss-trading.stockbit.com/ws"))
        .addMessageAdapterFactory(protobufMessageAdapterFactory)
        .addStreamAdapterFactory(coroutinesStreamAdapterFactory)
        .lifecycle(webSocketLifecycle)
        .build()
        .create(WebSocketTradingApi::class.java)
}

@Provides  
fun provideWebSocketTradingDataSource(api: WebSocketTradingApi): m0 {
    return WebSocketTradingDataSourceImpl(api)
}

@Provides
fun provideWebSocketTradingRepository(
    dataSource: m0,
    cacheDataSource: r,
    dispatchers: com.stockbit.features.model.a
): a {
    return WebSocketTradingRepositoryImpl(dispatchers, dataSource, cacheDataSource)
}
```

## Contoh Penggunaan Lengkap

### Subscribe ke Multiple Channels
```java
// Buat channel configuration
WebsocketChannel channel = WebsocketChannel.newBuilder()
    .addRunningTrade("BBCA")
    .addRunningTrade("TLKM")
    .addOrderBook("BBCA")
    .addBestBidOffer("BMRI")
    .setIsHotlist(true)
    .build();

// Buat request
WebsocketRequest request = WebsocketRequest.newBuilder()
    .setUserId(userId)
    .setKey(authKey)
    .setChannel(channel)
    .build();

// Kirim subscribe request
webSocketDataSource.sendSubscribe(request);
```

### Consume Running Trade Stream
```kotlin
webSocketDataSource.observeRunningTrade()
    .collect { runningTrade ->
        println("Stock: ${runningTrade.stock}")
        println("Price: ${runningTrade.price}")
        println("Volume: ${runningTrade.volume}")
        println("Action: ${runningTrade.action}")
    }
```

## Performance Considerations

1. **Batch Processing**: Gunakan `RunningTradeBatch` untuk mengurangi overhead message
2. **Selective Subscription**: Subscribe hanya ke channel yang diperlukan
3. **Connection Management**: Implement ping/pong untuk menjaga koneksi
4. **Error Handling**: Handle error code dengan tepat untuk reconnection

## Error Handling

| Error Code | Description | Action |
|------------|-------------|--------|
| UNAUTHORIZED | Key/session invalid | Re-authenticate |
| INVALID_REQUEST | Request malformed | Fix request format |
| CHANNEL_LIMIT_EXCEEDED | Too many subscriptions | Reduce subscribed channels |
| SERVER_ERROR | Server internal error | Retry with backoff |

## File Protobuf Lengkap

Berikut daftar file protobuf yang tersedia:

1. `WebsocketRequest.java` - Request subscribe
2. `WebsocketChannel.java` - Channel configuration
3. `RunningTrade.java` - Real-time trade data
4. `RunningTradeBatch.java` - Batch trade data
5. `Orderbook.java` - Order book data
6. `OrderBookHeader.java` - Order book header
7. `OrderBookBody.java` - Order book body
8. `Bid.java` - Bid data
9. `Offer.java` - Offer data
10. `BestBidOffer.java` - Best bid/offer
11. `Change.java` - Price change
12. `PingRequest.java` - Ping request
13. `PingResponse.java` - Ping response
14. `Error.java` - Error message
15. `ErrorCode.java` - Error codes
16. `TradeType.java` - Trade type enum
17. `BoardType.java` - Board type enum
18. `Hotlist.java` - Hotlist data
19. `Watchlist.java` - Watchlist data
20. `Top20.java` - Top 20 stocks

## Catatan Penting

1. Semua harga dalam format `double` (menggunakan presisi floating point)
2. Waktu menggunakan `google.protobuf.Timestamp`
3. Volume dalam satuan lot (1 lot = 100 lembar saham)
4. Untuk Indonesian stock market, harga dalam Rupiah (IDR)
5. Perubahan harga (`Change`) bisa positif/negatif

## Referensi File Penting

### Core WebSocket Implementation
1. **`com/stockbit/remote/api/securities/WebSocketTradingApi.java`** - Interface Scarlet WebSocket
2. **`com/stockbit/remote/datasource/WebSocketTradingDataSourceImpl.java`** - DataSource implementation
3. **`com/stockbit/repositories/websocket/trading/interactor/WebSocketTradingRepositoryImpl.java`** - Repository layer
4. **`com/stockbit/android/di/C4070b.java`** - DI Module dengan WebSocket configuration
5. **`com/stockbit/lib/appconfig/android/ui/AppConfigViewModel.java`** - AppConfig untuk endpoint management

### Protobuf Definitions
6. **`proto/securities/transactional/datafeed/v1/datafeed/datafeed.proto`** - Protobuf schema utama
7. **`com/stockbit/protobuf/securities/transactional/datafeed/v1/datafeed/`** - Semua generated Java classes:
   - `WebsocketRequest.java` - Request subscribe/unsubscribe
   - `WebsocketChannel.java` - Channel configuration
   - `RunningTrade.java` - Real-time trade data
   - `Orderbook.java` - Order book data
   - `BestBidOffer.java` - Best bid/offer data
   - `RunningTradeBatch.java` - Batch trade data
   - `PingRequest.java` / `PingResponse.java` - Keep-alive
   - `Error.java` / `ErrorCode.java` - Error handling

### Supporting Classes
8. **`com/stockbit/remote/datasource/WebSocketTradingDataSourceImpl$observeEvent$1.java`** - Event observer
9. **`com/stockbit/remote/datasource/WebSocketTradingDataSourceImpl$observeResponse$1.java`** - Response observer
10. **`com/stockbit/repositories/websocket/trading/interactor/WebSocketTradingRepositoryImpl$sendPayload$1.java`** - Payload sender
11. **`com/stockbit/repository/WebSocketProtobufRepositoryImpl.java`** - Legacy repository (jika ada)

### Lifecycle Management
12. **`com/stockbit/android/util/websocketlifecycle/`** - Lifecycle components:
   - `ConnectionAvailableLifecycle.java` - Network availability
   - `LoggedInLifecycle.java` - User authentication state
   - `ApplicationCreatedLifecycle.java` - App initialization

### DI Configuration
13. **`com/stockbit/android/b.java`** - Dagger/Koin provider untuk WebSocket
14. **`com/stockbit/android/di/m0.java`** - Additional DI utilities

## Summary

WebSocket trading Stockbit merupakan sistem real-time yang kompleks dengan arsitektur multi-layer:
1. **Transport**: `wss://wss-trading.stockbit.com/ws` dengan Protocol Buffers
2. **Client**: Scarlet Framework dengan reactive streams (Kotlin Flow)
3. **Architecture**: Clean Architecture dengan separation of concerns
4. **Lifecycle**: Custom lifecycle untuk optimal resource management
5. **Error Handling**: Comprehensive error codes dan retry mechanisms

Sistem ini mendukung berbagai channel data trading seperti running trade, order book, best bid/offer, dan market data lainnya dengan performa tinggi melalui batch processing dan selective subscription.

## Troubleshooting Guide

### Common Issues & Solutions
1. **Connection Timeout**: Check network availability dan ping/pong mechanism
2. **Authentication Failed**: Verify `key` field in `WebsocketRequest`
3. **Subscription Limit**: Reduce number of subscribed channels
4. **Data Not Streaming**: Check channel configuration dan lifecycle state
5. **Protobuf Decoding Error**: Verify message schema compatibility

### Monitoring & Debugging
- Log level 6: WebSocket connection events
- StateFlow for connection status monitoring
- Error codes dalam `Error` message
- Network troubleshooting integration

## Best Practices

1. **Subscribe selectively** hanya ke channel yang diperlukan
2. **Implement backoff strategy** untuk reconnection
3. **Monitor connection status** via `observeEvent()` stream
4. **Handle errors gracefully** dengan user-friendly messages
5. **Use batch channels** (`running_trade_batch`) untuk efisiensi
6. **Clean up subscriptions** ketika tidak diperlukan

## Version Compatibility

- **Protobuf Version**: v1 (securities.transactional.datafeed.v1)
- **Scarlet Version**: Compatible dengan coroutines flow
- **Android Min SDK**: Disesuaikan dengan aplikasi Stockbit
- **Network Protocol**: WebSocket dengan TLS encryption

## Security Considerations

1. **Authentication**: Key-based authentication via `WebsocketRequest.key`
2. **Encryption**: TLS/SSL pada WebSocket connection
3. **Authorization**: User-specific channel subscriptions
4. **Data Privacy**: Stock trading data protected

Dokumentasi ini diperbarui berdasarkan analisis codebase Stockbit Android tanggal 9 Januari 2026.
