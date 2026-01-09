# Login Flow HTTP Request Documentation

## Endpoints

### 1. Login dengan Username dan Password
- **Method**: POST
- **Endpoint**: `login/v5/username`
- **Header**:
  - `X-AppVersion`: "3.17.3"
  - `X-Platform`: "android"
  - `Accept-Language`: (sesuai bahasa perangkat)
  - `X-DeviceType`: (device model dari authRepository.p())
  - `Content-Type`: `application/json` (implisit dari Retrofit @a annotation)

- **Body** (`LoginUserNamePasswordDataParam`):
```json
{
  "user": "username",
  "password": "password",
  "player_id": "optional_player_id"
}
```

- **Response**: `SuccessResponse<LoginV5DTO>`

### 2. Login dengan Social Media (Facebook/Google)
- **Method**: POST
- **Endpoint**: `login/v5/social`
- **Header**: sama dengan endpoint di atas
- **Body** (`LoginSocialDataParam`):
```json
{
  "facebook": {
    "user_id": "facebook_id",
    "token": "facebook_token",
    "player_id": "optional_player_id",
    "token_type": "FACEBOOK_TOKEN_TYPE_ACCESS_TOKEN"
  },
  "google": {
    "google_id": "google_id",
    "token": "google_token",
    "player_id": "optional_player_id"
  }
}
```
Catatan: Hanya salah satu (facebook atau google) yang dikirim, tergantung provider.

- **Response**: `SuccessResponse<LoginV5DTO>`

### 3. Verifikasi Login di Device Baru
- **Method**: POST
- **Endpoint**: `login/v5/new-device/verify`
- **Header**: sama
- **Body** (`VerifyNewDeviceLoginDataParam`):
```json
{
  "multi_factor": {
    "login_token": "token_dari_response_login_sebelumnya"
  },
  "trusted_device": {
    "login_token": "login_token",
    "acknowledge_token": "acknowledge_token"
  }
}
```
Catatan: `multi_factor` atau `trusted_device` bisa dikirim tergantung flow.

- **Response**: `SuccessResponse<VerifyNewDeviceLoginDTO>`

### 4. Refresh Token
- **Method**: POST
- **Endpoint**: `login/refresh`
- **Header**:
  - `X-AppVersion`: "3.17.3"
  - `X-Platform`: "android"
  - `Authorization`: "Bearer <current_access_token>"
  - `X-DeviceType`: (device model)
  - `Accept-Language`: (sesuai bahasa)
- **Body**: `Map<String, String>` (kosong? sebenarnya parameter dikirim sebagai header juga)
- **Response**: `TokenResponse`

### 5. Logout
- **Method**: POST
- **Endpoint**: `logout` (main auth) dan `auth/logout` (securities)
- **Header**: 
  - `X-AppVersion`: "3.17.3"
  - `X-Platform`: "android"
  - `Accept-Language`: (sesuai bahasa perangkat)
  - `X-DeviceType`: (device model)
  - `Authorization`: "Bearer <current_access_token>" (wajib, karena logout membutuhkan autentikasi)
- **Body**: tidak ada (empty body)
- **Response**: 
  - Untuk `logout`: `BaseResponseImpl`
  - Untuk `auth/logout` (securities): `StockbitSecuritiesBaseResponseImpl`

### 6. Logout Securities
- **Method**: POST
- **Endpoint**: `auth/logout`
- **Header**: sama dengan header umum + Authorization
- **Body**: tidak ada
- **Response**: `StockbitSecuritiesBaseResponseImpl`

### 7. Multi‑Factor Authentication
- **Method**: POST (melalui endpoint verifyNewDevice dengan MultiFactorDataParam)
- **Body** (`MultiFactorDataParam`):
```json
{
  "login_token": "token"
}
```

## Header Umum untuk Semua Request

Semua request ke backend Stockbit dilengkapi dengan header berikut (melalui interceptor):

| Header | Nilai | Sumber |
|--------|-------|--------|
| `X-AppVersion` | "3.17.3" | Hardcoded di interceptor |
| `X-Platform` | "android" | Hardcoded |
| `Accept-Language` | nilai dari `AcceptLanguageType` | Diambil dari pengaturan bahasa perangkat |
| `X-DeviceType` | string device model | `authRepository.p()` |
| `Authorization` | "Bearer <token>" | Hanya untuk request yang membutuhkan auth token |

## Flow Logout

### E. Logout
1. User memilih logout dari aplikasi
2. Client memanggil endpoint `logout` (dan juga `auth/logout` untuk securities jika diperlukan)
3. Request header menyertakan `Authorization: Bearer <current_access_token>` yang masih valid
4. Server mencabut session dan menghapus token dari daftar aktif
5. Client menghapus token lokal dan mengarahkan user ke halaman login
6. Jika logout berhasil, server mengembalikan `BaseResponseImpl` dengan status sukses

## Flow Login Lengkap

### A. Login Username/Password
1. User memasukkan username & password
2. Client membuat `LoginUserNamePasswordDataParam`
3. Request POST ke `login/v5/username`
4. Jika sukses, dapat `LoginV5DTO` berisi access token, refresh token, dan user data.
5. Jika device belum terdaftar (new device), akan ada response yang meminta verifikasi via OTP atau trusted device.

### B. Login Social Media
1. User memilih provider (Facebook/Google)
2. Client menerima token dari provider SDK
3. Client membuat `LoginFacebookDataParam` atau `LoginGoogleDataParam`
4. Masukkan ke `LoginSocialDataParam`
5. Request POST ke `login/v5/social`
6. Response sama seperti login username/password.

### C. Verifikasi Device Baru
1. Setelah login berhasil tetapi device baru terdeteksi, server mengembalikan status yang mengharuskan verifikasi.
2. Client mengirim `VerifyNewDeviceLoginDataParam` dengan `multi_factor` (untuk OTP) atau `trusted_device` (jika device sudah pernah dipercaya).
3. Request POST ke `login/v5/new-device/verify`
4. Jika sukses, user mendapatkan akses penuh.

### D. Refresh Token
1. Ketika access token expire (HTTP 401), interceptor (`AuthInterceptorHelper`) otomatis memanggil endpoint `login/refresh` dengan refresh token yang tersimpan.
2. Request header menyertakan `Authorization: Bearer <current_access_token>` (yang sudah expire) dan header lainnya.
3. Server mengembalikan `TokenResponse` dengan token baru.
4. Interceptor mengupdate token di lokal dan mengulang request original.

## Catatan Implementasi

- Semua request dilakukan melalui Retrofit dengan suspend function (Kotlin Coroutines).
- Interceptor (`AuthInterceptorStockbitSecurities`, `AuthInterceptorHelper`) menambahkan header secara otomatis.
- Error handling menggunakan `ErrorResponse` wrapper.
- Untuk keamanan, token disimpan secara aman dan refresh dilakukan dengan mutex untuk menghindari race condition.

## File‑file Penting

- `com/stockbit/remote/api/AuthApi.java` – Interface Retrofit untuk semua endpoint auth.
- `com/stockbit/datasource/param/login/*` – Class parameter request body.
- `com/stockbit/remote/utils/AuthInterceptorHelper.java` – Helper untuk refresh token.
- `com/stockbit/remote/di/AuthInterceptorStockbitSecurities.java` – Interceptor untuk menambahkan header umum.
- `com/stockbit/remote/datasource/RemoteLoginDataSourceImpl.java` – Implementasi data source yang memanggil API.

## Contoh Request (cURL)

### Login Username/Password
```bash
curl -X POST 'https://api.stockbit.com/login/v5/username' \
  -H 'X-AppVersion: 3.17.3' \
  -H 'X-Platform: android' \
  -H 'Accept-Language: en-US' \
  -H 'X-DeviceType: Pixel 5' \
  -H 'Content-Type: application/json' \
  -d '{"user":"user@example.com","password":"password123","player_id":null}'
```

### Refresh Token
```bash
curl -X POST 'https://api.stockbit.com/login/refresh' \
  -H 'X-AppVersion: 3.17.3' \
  -H 'X-Platform: android' \
  -H 'Authorization: Bearer <current_access_token>' \
  -H 'X-DeviceType: Pixel 5' \
  -H 'Accept-Language: en-US'
```

---

*Dokumentasi ini dibuat berdasarkan analisis kodebase Stockbit Android.*