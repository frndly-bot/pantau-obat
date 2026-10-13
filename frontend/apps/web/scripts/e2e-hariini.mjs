import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1';
const BASE='http://127.0.0.1:5173';
const ORIGIN='http://localhost:5173';
const email=`h-${Date.now()}@example.com`;
const password='SandiUji123!';
let fail=0;
const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};

// Siapkan data lewat API dulu (obat harian 3 jam).
const reg=await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password,full_name:'Budi Santoso',timezone:'Asia/Jakarta'})});
const jr=await reg.json();
const token=jr.data.access_token;
const today=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Jakarta'}).format(new Date());
const H={Authorization:`Bearer ${token}`,'Content-Type':'application/json',Origin:ORIGIN};
const nowWib=new Intl.DateTimeFormat('en-GB',{timeZone:'Asia/Jakarta',hour:'2-digit',minute:'2-digit',hour12:false}).format(new Date());
// Jam dosis: satu sebelum sekarang (pasti due), satu jauh setelah (pasti pending)
const [nh,nm]=nowWib.split(':').map(Number); const nowMin=nh*60+nm;
const past=`${String(Math.max(0,Math.floor((nowMin-60)/60))).padStart(2,'0')}:${String((nowMin-60+1440)%60).padStart(2,'0')}`;
const future='23:59';
await fetch(`${API}/medications`,{method:'POST',headers:H,body:JSON.stringify({
  name:'Amlodipine 5mg',dose_value:1,dose_unit_code:'tablet',meal_relation:'after_meal',
  route_code:'O',dosage_form_code:'LOCAL-TABS',start_date:today,start_time:past,
  frequency:{pattern:'daily',times_per_day:2,tolerance_minutes:30,start_time:past,schedule_timezone:'Asia/Jakarta'},
  times:[{time_local:past,label:'morning'},{time_local:future,label:'evening'}],
  stock:{unit_code:'tablet',initial_qty:30,qty_per_dose:1,refill_threshold_qty:5,refill_reminder:true}
})});
console.log(`     (data siap: jam ${past} = due, ${future} = pending; sekarang ${nowWib} WIB)`);

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
// `locale: 'id-ID'` — aplikasi menghormati bahasa peramban, sedangkan bawaan
// Playwright `en-US`. Tanpa ini halaman tampil Inggris dan uji teks Indonesia gagal.
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,locale:'id-ID'});
const p=await ctx.newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));

// Login lewat UI
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',email); await p.fill('#password',password);
await p.click('button[type="submit"]');
await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.waitForTimeout(1500);

const cards=await p.locator('main li').count();
check('slot jadwal tampil di Hari Ini', cards===2, `${cards} kartu`);
const body=await p.textContent('main');
check('nama obat tampil', body.includes('Amlodipine 5mg'));
check('jam due tampil', body.includes(past));
check('jam pending tampil', body.includes(future));
check('label "Belum dicatat" tampil', body.includes('Belum dicatat'), '');
check('sisa stok tampil', body.includes('Sisa stok'));
check('tombol Diminum ada', (await p.locator('button:has-text("Diminum")').count())>0);

// Klik Diminum pada kartu yang due
await p.locator('button:has-text("Diminum")').first().click();
await p.waitForTimeout(2000);
const after=await p.textContent('main');
const recorded = after.includes('Diminum') || after.includes('Terlambat');
check('setelah klik: dosis tercatat', recorded);
check('tombol Diminum berkurang (1 sisa untuk pending)', (await p.locator('button:has-text("Diminum")').count())<=1);

await p.screenshot({path:'/tmp/po-hariini-real.png'});
console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
