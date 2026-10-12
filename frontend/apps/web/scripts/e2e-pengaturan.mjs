import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1', BASE='http://127.0.0.1:5173', ORIGIN='http://localhost:5173';
const email=`set-${Date.now()}@example.com`, password='SandiUji123!';
let fail=0; const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};

const tok=(await (await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password,full_name:'Uji Pengaturan',timezone:'Asia/Jakarta'})})).json()).data.access_token;
// Buat sedikit aktivitas agar audit-log ada isinya
await fetch(`${API}/medications`,{method:'POST',headers:{Authorization:`Bearer ${tok}`,'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({name:'Obat Audit',dose_value:1,dose_unit_code:'tablet',route_code:'O',meal_relation:'after_meal',start_date:new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Jakarta'}).format(new Date()),start_time:'08:00',frequency:{pattern:'daily',times_per_day:1,tolerance_minutes:60,start_time:'08:00',schedule_timezone:'Asia/Jakarta'},times:[{time_local:'08:00',label:'morning'}]})});

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
const p=await (await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2})).newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',email); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.goto(BASE+'/pengaturan',{waitUntil:'networkidle'}); await p.waitForTimeout(1800);

const t=await p.textContent('main');
check('halaman pengaturan memuat', (await p.textContent('h1'))?.includes('Pengaturan'));
check('profil: email tampil', t?.includes(email));
check('profil: nama tampil', t?.includes('Uji Pengaturan'));
check('kanal notifikasi dari backend', t?.includes('Dalam aplikasi'));
check('kanal lain bertanda Nanti', t?.includes('Nanti'));
check('riwayat aktivitas ada isinya', /Menambah data \(obat\)|Membuat|Menambah/.test(t ?? ''), (t ?? '').match(/Menambah[^\n]{0,25}/)?.[0] ?? 'tidak ada');
check('catatan privasi audit tampil', t?.includes('Nama obat tidak pernah masuk riwayat'));
check('tombol Ganti sandi ada', (await p.locator('button:has-text("Ganti sandi")').count())>0);

// Ganti sandi: validasi tombol
await p.locator('button:has-text("Ganti sandi")').click(); await p.waitForTimeout(600);
check('dialog ganti sandi terbuka', (await p.textContent('body'))?.includes('Sandi sekarang'));
const dis1=await p.locator('button:has-text("Simpan")').last().isDisabled();
check('Simpan nonaktif saat kosong', dis1);
await p.fill('#pw-cur',password); await p.fill('#pw-new','SandiBaru456!'); await p.fill('#pw-rep','BEDA456!');
await p.waitForTimeout(300);
check('Simpan nonaktif saat ulangan beda', await p.locator('button:has-text("Simpan")').last().isDisabled());
await p.fill('#pw-rep','SandiBaru456!'); await p.waitForTimeout(300);
await p.locator('button:has-text("Simpan")').last().click(); await p.waitForTimeout(2500);
check('pesan sukses ganti sandi', (await p.textContent('main'))?.includes('Sandi diganti'));

// Sesi lain dicabut → login ulang dengan sandi BARU harus berhasil
const r=await fetch(`${API}/auth/login`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password:'SandiBaru456!'})});
check('login dengan sandi baru berhasil', r.status===200, `status ${r.status}`);
const r2=await fetch(`${API}/auth/login`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password})});
check('login dengan sandi lama gagal', r2.status===401, `status ${r2.status}`);

// Hapus akun: pakai AKUN TERPISAH.
// Mengganti sandi mencabut seluruh sesi lain (termasuk sesi peramban ini),
// jadi menghapus akun dari sesi yang sama akan gagal karena tokennya sudah
// mati — bukan karena fiturnya rusak.
const delEmail=`del-${Date.now()}@example.com`;
await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email:delEmail,password,full_name:'Uji Hapus',timezone:'Asia/Jakarta'})});
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',delEmail); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.goto(BASE+'/pengaturan',{waitUntil:'networkidle'}); await p.waitForTimeout(1500);
await p.locator('button:has-text("Hapus akun saya")').click(); await p.waitForTimeout(600);
check('dialog hapus akun terbuka', (await p.textContent('body'))?.includes('Ketik HAPUS'));
check('tombol hapus nonaktif tanpa konfirmasi', await p.locator('button:has-text("Hapus akun")').last().isDisabled());
await p.fill('#del-confirm','HAPUS'); await p.waitForTimeout(300);
await p.locator('button:has-text("Hapus akun")').last().click(); await p.waitForTimeout(3000);
check('setelah hapus: keluar dari pengaturan', !p.url().includes('pengaturan'), p.url());
const r3=await fetch(`${API}/auth/login`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email:delEmail,password})});
check('akun terhapus tidak bisa login lagi', r3.status===401, `status ${r3.status}`);

await p.screenshot({path:'/tmp/po-pengaturan.png'});
console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
