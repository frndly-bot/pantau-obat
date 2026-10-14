import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1', BASE='http://127.0.0.1:5173', ORIGIN='http://localhost:5173';
const email=`det-${Date.now()}@example.com`, password='SandiUji123!';
let fail=0; const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};

const reg=await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password,full_name:'Uji Detail',timezone:'Asia/Jakarta'})});
const tok=(await reg.json()).data.access_token;
const today=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Jakarta'}).format(new Date());
// Buat obat dengan stok
const c=await fetch(`${API}/medications`,{method:'POST',headers:{Authorization:`Bearer ${tok}`,'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({
  name:'Metformin 500mg',dose_value:1,dose_unit_code:'tablet',route_code:'O',dosage_form_code:'LOCAL-TABS',meal_relation:'after_meal',
  start_date:today,start_time:'08:00',
  frequency:{pattern:'weekly',days_of_week:9,tolerance_minutes:60,start_time:'08:00',schedule_timezone:'Asia/Jakarta'},
  times:[{time_local:'08:00',label:'morning'}],
  stock:{unit_code:'tablet',initial_qty:60,qty_per_dose:1,refill_threshold_qty:10,refill_reminder:true}
})});
const med=(await c.json()).data;
check('obat dibuat dengan stok', c.status===201, `status ${c.status}`);

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
const p=await (await b.newContext({ viewport: {width:390,height:844}, locale: 'id-ID',deviceScaleFactor:2})).newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',email); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});

// Daftar → klik kartu → detail
await p.goto(BASE+'/obat',{waitUntil:'networkidle'}); await p.waitForTimeout(800);
await p.locator('a[href^="/obat/"]').first().click();
await p.waitForURL(/\/obat\/[0-9a-f-]+/,{timeout:15000}).catch(()=>{});
await p.waitForTimeout(1200);
check('berpindah ke halaman detail', /\/obat\/[0-9a-f-]+/.test(p.url()), p.url());
const t=await p.textContent('main');
check('nama obat tampil', t?.includes('Metformin 500mg'));
check('pola mingguan tampil', t?.includes('Hari tertentu'));
check('hari Senin+Kamis tampil', t?.includes('Sen') && t?.includes('Kam'), '');
check('jam minum tampil', t?.includes('08:00'));
check('stok tampil', t?.includes('60'));
check('perkiraan habis atau belum cukup data', t?.includes('Perkiraan habis'));
check('tombol Isi ulang ada', (await p.locator('button:has-text("Isi ulang")').count())>0);

// Isi ulang
await p.locator('button:has-text("Isi ulang")').click(); await p.waitForTimeout(600);
await p.fill('#r-qty','30'); await p.waitForTimeout(200);
await p.locator('button:has-text("Simpan")').click(); await p.waitForTimeout(2200);
const t2=await p.textContent('main');
check('saldo jadi 90 setelah isi ulang 30', t2?.includes('90'), '');
check('riwayat stok menampilkan isi ulang', t2?.includes('Isi ulang'));

// Koreksi saldo — alasan wajib
await p.locator('button:has-text("Koreksi saldo")').click(); await p.waitForTimeout(600);
const saveDisabled=await p.locator('button:has-text("Simpan")').last().isDisabled();
check('tombol Simpan koreksi nonaktif tanpa alasan', saveDisabled);
await p.fill('#a-qty','-2'); await p.fill('#a-note','tablet pecah'); await p.waitForTimeout(200);
await p.locator('button:has-text("Simpan")').last().click(); await p.waitForTimeout(2200);
const t3=await p.textContent('main');
check('saldo jadi 88 setelah koreksi -2', t3?.includes('88'), '');

await p.screenshot({path:'/tmp/po-detail-obat.png'});
console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
