import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1', BASE='http://127.0.0.1:5173', ORIGIN='http://localhost:5173';
const email=`riw-${Date.now()}@example.com`, password='SandiUji123!';
let fail=0; const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};
const today=new Intl.DateTimeFormat('en-CA',{timeZone:'Asia/Jakarta'}).format(new Date());
const nowWib=new Intl.DateTimeFormat('en-GB',{timeZone:'Asia/Jakarta',hour:'2-digit',minute:'2-digit',hour12:false}).format(new Date());
const [nh,nm]=nowWib.split(':').map(Number);
const past=`${String(Math.max(0,Math.floor((nh*60+nm-90)/60))).padStart(2,'0')}:${String((nh*60+nm-90+1440)%60).padStart(2,'0')}`;

const tok=(await (await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password,full_name:'Uji Riwayat',timezone:'Asia/Jakarta'})})).json()).data.access_token;
const H={Authorization:`Bearer ${tok}`,'Content-Type':'application/json',Origin:ORIGIN};
const c=await fetch(`${API}/medications`,{method:'POST',headers:H,body:JSON.stringify({name:'Amlodipine 10mg',dose_value:1,dose_unit_code:'tablet',route_code:'O',dosage_form_code:'LOCAL-TABS',meal_relation:'after_meal',start_date:today,start_time:past,frequency:{pattern:'daily',times_per_day:1,tolerance_minutes:60,start_time:past,schedule_timezone:'Asia/Jakarta'},times:[{time_local:past,label:'morning'}],stock:{unit_code:'tablet',initial_qty:30,qty_per_dose:1,refill_threshold_qty:5,refill_reminder:true}})});
const med=(await c.json()).data;
// Catat satu dosis agar riwayat berisi
const sched=(await (await fetch(`${API}/schedule`,{headers:H})).json()).data;
const slot=sched.find(s=>s.medication_id===med.id);
const rec=await fetch(`${API}/medications/${med.id}/logs`,{method:'POST',headers:H,body:JSON.stringify({schedule_id:slot.schedule_id,slot_date:today,scheduled_hour:slot.scheduled_hour,scheduled_minute:slot.scheduled_minute,scheduled_tz:'Asia/Jakarta',status:'taken'})});
check('dosis tercatat lewat API', rec.status===201, `status ${rec.status}`);

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
const ctx=await b.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,acceptDownloads:true});
const p=await ctx.newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',email); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});

await p.goto(BASE+'/riwayat',{waitUntil:'networkidle'}); await p.waitForTimeout(1500);
const t=await p.textContent('main');
check('riwayat memuat', (await p.textContent('h1'))?.includes('Riwayat'));
check('nama obat tampil (dipetakan dari daftar)', t?.includes('Amlodipine 10mg'), '');
check('status Diminum tampil', t?.includes('Diminum'));
check('filter rentang ada', (await p.locator('#f-from').count())===1 && (await p.locator('#f-to').count())===1);
check('tombol preset 30 hari ada', (await p.locator('button:has-text("30 hari")').count())>0);

// Unduh CSV sungguhan
const dl = p.waitForEvent('download', {timeout:20000});
await p.locator('button:has-text("Ekspor CSV")').click();
const download = await dl.catch(()=>null);
check('unduhan CSV terjadi', !!download, download? await download.suggestedFilename() : 'tidak ada unduhan');
if (download) {
  const path = await download.path();
  const fs = await import('node:fs');
  const content = fs.readFileSync(path,'utf8');
  check('CSV punya header', content.split('\n')[0].includes('slot_date') || content.split('\n')[0].includes('tanggal'), content.split('\n')[0].slice(0,60));
  check('CSV berisi catatan hari ini', content.includes(today), '');
  check('CSV punya BOM (Excel Indonesia aman)', content.charCodeAt(0)===0xFEFF, `charCode ${content.charCodeAt(0).toString(16)}`);
  console.log('     ukuran CSV:', content.length, 'byte');
}

// Filter status. CATATAN: teks "Diminum" selalu ada di layar karena tombol
// filter berlabel itu, jadi yang diperiksa adalah daftar catatannya — nama
// obat harus hilang setelah disaring ke "Dilewati" (catatan kita statusnya
// taken, bukan skipped).
await p.locator('button:has-text("Dilewati")').click(); await p.waitForTimeout(1800);
const listArea = await p.locator('main section').count();
const hasMed = (await p.textContent('main'))?.includes('Amlodipine 10mg');
check('filter Dilewati menyaring catatan (nama obat hilang)', !hasMed, `${listArea} bagian`);
// Kembali ke Semua harus memunculkan lagi.
// Pencocokan HARUS persis: label "Semua sumber" juga mengandung "Semua",
// sehingga `has-text` biasa akan cocok dengan dua tombol sekaligus.
await p.getByRole('button', { name: 'Semua', exact: true }).click(); await p.waitForTimeout(1800);
check('filter Semua memunculkan catatan lagi', (await p.textContent('main'))?.includes('Amlodipine 10mg'), '');

// ── Filter sumber ──
// Sumber yang tersedia ada TIGA (self, caregiver, system), bukan dua seperti
// di prototipe design. Catatan yang baru dibuat lewat API bersumber `self`.
check('tombol filter sumber "Sistem" ada', (await p.getByRole('button', { name: 'Sistem', exact: true }).count()) === 1, '');
check('tombol filter sumber "Caregiver" ada', (await p.getByRole('button', { name: 'Caregiver', exact: true }).count()) === 1, '');

await p.getByRole('button', { name: 'Saya sendiri', exact: true }).click(); await p.waitForTimeout(1800);
check('filter sumber "Saya sendiri" tetap menampilkan catatan', (await p.textContent('main'))?.includes('Amlodipine 10mg'), '');

await p.getByRole('button', { name: 'Sistem', exact: true }).click(); await p.waitForTimeout(1800);
check('filter sumber "Sistem" menyaring catatan buatan manusia', !(await p.textContent('main'))?.includes('Amlodipine 10mg'), '');

await p.getByRole('button', { name: 'Semua sumber', exact: true }).click(); await p.waitForTimeout(1800);
check('kembali ke "Semua sumber" memunculkan catatan lagi', (await p.textContent('main'))?.includes('Amlodipine 10mg'), '');

await p.screenshot({path:'/tmp/po-riwayat.png'});
console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
