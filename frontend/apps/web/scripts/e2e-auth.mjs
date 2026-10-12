// Verifikasi alur autentikasi memakai fetch + cookie jar sederhana.
// Meniru yang dilakukan browser: register/login (simpan cookie), lalu
// memanggil endpoint terproteksi dengan Bearer, lalu refresh & logout.
const API = process.env.API ?? 'http://127.0.0.1:8081/api/v1';
const ORIGIN = 'http://localhost:5173';
const email = `e2e-${Date.now()}@example.com`;
const password = 'SandiUji123!';

let cookie = '';
function captureCookies(res) {
	const sc = res.headers.getSetCookie?.() ?? [];
	for (const c of sc) cookie += (cookie ? '; ' : '') + c.split(';')[0];
}

let failures = 0;
function check(label, ok, detail = '') {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) failures++;
}

// 1. Register
const r1 = await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'E2E Uji', timezone: 'Asia/Jakarta' }),
});
captureCookies(r1);
const j1 = await r1.json();
check('register 201 + access_token', r1.status === 201 && !!j1.data?.access_token, `status ${r1.status}`);
check('cookie po_refresh diterima', cookie.includes('po_refresh'));

// 2. /auth/me dengan token
const r2 = await fetch(`${API}/auth/me`, {
	headers: { Authorization: `Bearer ${j1.data.access_token}`, Origin: ORIGIN },
});
const j2 = await r2.json();
check('/auth/me 200 + email cocok', r2.status === 200 && j2.data?.email === email, `status ${r2.status}`);

// 3. Envelope { data } — memastikan bentuk balasan yang diharapkan klien
check('balasan memakai envelope { data }', j2.data !== undefined && j2.error === undefined);

// 4. Refresh memakai cookie
const r3 = await fetch(`${API}/auth/refresh`, { method: 'POST', headers: { Origin: ORIGIN, Cookie: cookie } });
const j3 = await r3.json();
check('refresh 200 + token baru', r3.status === 200 && !!j3.data?.access_token, `status ${r3.status}`);

// 5. Endpoint terproteksi
const r4 = await fetch(`${API}/logs/today`, {
	headers: { Authorization: `Bearer ${j3.data.access_token}`, Origin: ORIGIN },
});
check('/logs/today 200', r4.status === 200, `status ${r4.status}`);

// 6. Tanpa token harus 401
const r5 = await fetch(`${API}/logs/today`, { headers: { Origin: ORIGIN } });
check('/logs/today tanpa token → 401', r5.status === 401, `status ${r5.status}`);

// 7. Logout (butuh Bearer — endpoint ini terproteksi), lalu refresh harus gagal
const r6 = await fetch(`${API}/auth/logout`, {
	method: 'POST',
	headers: { Authorization: `Bearer ${j3.data.access_token}`, Origin: ORIGIN, Cookie: cookie },
});
check('logout 2xx', r6.status >= 200 && r6.status < 300, `status ${r6.status}`);
const r7 = await fetch(`${API}/auth/refresh`, { method: 'POST', headers: { Origin: ORIGIN, Cookie: cookie } });
check('refresh setelah logout → 401', r7.status === 401, `status ${r7.status}`);

console.log(failures === 0 ? '\nSEMUA LULUS' : `\n${failures} GAGAL`);
process.exit(failures === 0 ? 0 : 1);
