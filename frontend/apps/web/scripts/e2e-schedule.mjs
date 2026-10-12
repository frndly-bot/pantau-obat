// Menguji GET /api/v1/schedule dengan data nyata.
//
// Skenario: daftar user, buat obat pola HARIAN dengan 3 jam (satu sudah lewat,
// satu baru lewat, satu belum), lalu pastikan endpoint mengembalikan 3 slot
// dengan status yang benar. Setelah mencatat satu dosis, slot itu harus
// berubah menjadi `taken`.
const API = 'http://127.0.0.1:8081/api/v1';
const ORIGIN = 'http://localhost:5173';

let failures = 0;
function check(label, ok, detail = '') {
	console.log(`${ok ? 'OK  ' : 'GAGAL'} ${label}${detail ? ' — ' + detail : ''}`);
	if (!ok) failures++;
}

const email = `sched-${Date.now()}@example.com`;
const password = 'SandiUji123!';

const reg = await fetch(`${API}/auth/register`, {
	method: 'POST',
	headers: { 'Content-Type': 'application/json', Origin: ORIGIN },
	body: JSON.stringify({ email, password, full_name: 'Uji Jadwal', timezone: 'Asia/Jakarta' }),
});
const jr = await reg.json();
const token = jr.data?.access_token;
check('register', reg.status === 201 && !!token, `status ${reg.status}`);
const H = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', Origin: ORIGIN };

// Tanggal hari ini menurut WIB.
const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta' }).format(new Date());

// Buat obat harian 3x sehari. Jam dikirim SEKALIGUS di sini — backend
// memvalidasi jumlah jam terhadap times_per_day pada saat pembuatan.
//
// route_code memakai kode dari tabel ref_route (bukan nama): "O" = Oral.
// dosage_form_code memakai kode ref_dosage_form. Nilai yang salah ditolak
// oleh foreign key, jadi harus tepat.
const createBody = {
	name: 'Amlodipine 5mg',
	dose_value: 1,
	dose_unit_code: 'tablet',
	meal_relation: 'after_meal',
	route_code: 'O',
	dosage_form_code: 'LOCAL-TABS',
	start_date: today,
	start_time: '07:00',
	frequency: {
		pattern: 'daily',
		times_per_day: 3,
		tolerance_minutes: 30,
		start_time: '07:00',
		schedule_timezone: 'Asia/Jakarta',
	},
	times: [
		{ time_local: '07:00', label: 'morning' },
		{ time_local: '13:00', label: 'afternoon' },
		{ time_local: '23:00', label: 'evening' },
	],
	stock: { unit_code: 'tablet', initial_qty: 30, qty_per_dose: 1, refill_threshold_qty: 5, refill_reminder: true },
};
const c = await fetch(`${API}/medications`, { method: 'POST', headers: H, body: JSON.stringify(createBody) });
const jc = await c.json();
check('buat obat harian 3 jam', c.status === 201 || c.status === 200, `status ${c.status} ${JSON.stringify(jc).slice(0,150)}`);
const medId = jc.data?.id;

// Uji endpoint jadwal
const s = await fetch(`${API}/schedule`, { headers: H });
const js = await s.json();
check('GET /schedule 200', s.status === 200, `status ${s.status}`);
const slots = js.data ?? [];
check('slot ditemukan', slots.length === 3, `${slots.length} slot`);
if (slots.length) {
  const times = slots.map((x) => x.time_local).join(', ');
  check('jam sesuai (HH:MM, tanpa detik)', times === '07:00, 13:00, 23:00', times);
  check('ada schedule_id', slots.every((x) => !!x.schedule_id));
  check('ada nama obat', slots.every((x) => x.medication_name === 'Amlodipine 5mg'));
  check('ada sisa stok', slots.every((x) => typeof x.stock_remaining === 'number'));
  const statuses = slots.map((x) => `${x.time_local}=${x.status}`).join(' ');
  console.log(`     status: ${statuses}`);
  // Status bergantung jam sekarang (WIB), jadi diuji secara logis:
  // dosis yang jamnya belum lewat harus "pending", yang sudah lewat "due".
  const nowWib = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Asia/Jakarta', hour: '2-digit', minute: '2-digit', hour12: false,
  }).format(new Date());
  const [nh, nm] = nowWib.split(':').map(Number);
  const nowMin = nh * 60 + nm;
  let statusOk = true;
  for (const s of slots) {
    const [h, m] = s.time_local.split(':').map(Number);
    const expected = h * 60 + m > nowMin ? 'pending' : 'due';
    if (s.status !== expected) {
      statusOk = false;
      console.log(`     (${s.time_local}: dapat "${s.status}", seharusnya "${expected}" — sekarang ${nowWib} WIB)`);
    }
  }
  check(`status sesuai waktu sekarang (${nowWib} WIB)`, statusOk);
}

// Catat dosis 07:00 → slot harus berubah dari "due" menjadi status catatan.
const seven = slots.find((x) => x.time_local === '07:00');
if (seven) {
  const r = await fetch(`${API}/medications/${medId}/logs`, {
    method: 'POST', headers: H,
    body: JSON.stringify({
      schedule_id: seven.schedule_id,
      slot_date: today,
      scheduled_hour: seven.scheduled_hour,
      scheduled_minute: seven.scheduled_minute,
      scheduled_tz: 'Asia/Jakarta',
      status: 'taken',
    }),
  });
  const jl = await r.json();
  check('catat dosis 07:00', r.status === 201 || r.status === 200, `status ${r.status} ${JSON.stringify(jl).slice(0,140)}`);

  const s2 = await fetch(`${API}/schedule`, { headers: H });
  const js2 = await s2.json();
  const after = (js2.data ?? []).find((x) => x.time_local === '07:00');
  // Backend yang menentukan status akhir: mencatat di luar jendela toleransi
  // menghasilkan "late", bukan "taken". Yang penting di sini adalah slot TIDAK
  // lagi "due"/"pending" dan sudah punya log_id.
  const recorded = after && !['due', 'pending'].includes(after.status);
  check(`slot 07:00 tidak lagi "due" (jadi "${after?.status}")`, recorded, `status ${after?.status}`);
  check('slot punya log_id', !!after?.log_id);

  // Catat ulang dosis yang sama harus ditolak — anti-ganda lewat indeks unik.
  const dup = await fetch(`${API}/medications/${medId}/logs`, {
    method: 'POST', headers: H,
    body: JSON.stringify({
      schedule_id: seven.schedule_id, slot_date: today,
      scheduled_hour: seven.scheduled_hour, scheduled_minute: seven.scheduled_minute,
      scheduled_tz: 'Asia/Jakarta', status: 'taken',
    }),
  });
  const jdup = await dup.json();
  check('catat ulang dosis sama → 409 LOG_OCCURRENCE_TAKEN',
    dup.status === 409 && jdup.error?.code === 'LOG_OCCURRENCE_TAKEN',
    `status ${dup.status} code ${jdup.error?.code}`);
}

// Tanpa token harus 401
const s3 = await fetch(`${API}/schedule`, { headers: { Origin: ORIGIN } });
check('tanpa token → 401', s3.status === 401, `status ${s3.status}`);

console.log(failures === 0 ? '\nSEMUA LULUS' : `\n${failures} GAGAL`);
process.exit(failures === 0 ? 0 : 1);
