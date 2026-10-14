import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1';
const BASE='http://127.0.0.1:5173';
const ORIGIN='http://localhost:5173';
const password='SandiUji123!';
let fail=0;
const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};

const email=`wiz-${Date.now()}@example.com`;
await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password,full_name:'Uji Wizard',timezone:'Asia/Jakarta'})});

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
const ctx=await b.newContext({ viewport: {width:390,height:844}, locale: 'id-ID',deviceScaleFactor:2});
const p=await ctx.newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));

await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',email); await p.fill('#password',password);
await p.click('button[type="submit"]');
await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.goto(BASE+'/obat',{waitUntil:'networkidle'});
await p.waitForTimeout(800);

/** Menyelesaikan wizard untuk satu pola. */
async function addViaWizard({name, patternLabel, steps}) {
  await p.locator('button:has-text("Tambah obat")').first().click();
  await p.waitForTimeout(1000);
  await p.fill('#m-name', name);
  await p.locator('button:has-text("Lanjut")').click();   // → detail
  await p.waitForTimeout(300);
  await p.locator('button:has-text("Lanjut")').click();   // → pola
  await p.waitForTimeout(300);
  await p.locator(`button:has-text("${patternLabel}")`).first().click();
  await p.waitForTimeout(400);
  if (steps) await steps();
  await p.locator('button:has-text("Lanjut")').click();   // → jam
  await p.waitForTimeout(300);
  await p.locator('button:has-text("Lanjut")').click();   // → stok
  await p.waitForTimeout(300);
  await p.locator('button:has-text("Simpan obat")').click();
  await p.waitForTimeout(2200);
}

// 1. daily — default 2 jam
await addViaWizard({name:'Obat Harian', patternLabel:'Sekian kali sehari'});
check('pola harian tersimpan', (await p.textContent('main'))?.includes('Obat Harian'));

// 2. interval
await addViaWizard({name:'Obat Interval', patternLabel:'Tiap sekian jam'});
check('pola interval tersimpan', (await p.textContent('main'))?.includes('Obat Interval'));

// 3. weekly — pilih hari
await addViaWizard({name:'Obat Mingguan', patternLabel:'Hari tertentu', steps: async () => {
  // Default semua hari menyala, jadi Kosongkan dulu lalu pilih Sen+Kam.
  // Ini juga menguji tombol hari benar-benar mengubah bitmask (bukan hanya tampil).
  await p.getByRole('button',{name:'Kosongkan',exact:true}).click();
  await p.waitForTimeout(250);
  await p.getByRole('button',{name:'Sen',exact:true}).click();
  await p.waitForTimeout(150);
  await p.getByRole('button',{name:'Kam',exact:true}).click();
  await p.waitForTimeout(200);
}});
check('pola mingguan tersimpan', (await p.textContent('main'))?.includes('Obat Mingguan'));

// 4. monthly_cycle
await addViaWizard({name:'Obat Siklus', patternLabel:'Siklus minum–jeda'});
check('pola siklus tersimpan', (await p.textContent('main'))?.includes('Obat Siklus'));

// 5. PRN
await addViaWizard({name:'Obat PRN', patternLabel:'Bila perlu (PRN)'});
check('pola PRN tersimpan', (await p.textContent('main'))?.includes('Obat PRN'));

// Semua harus muncul di daftar
const list=await p.textContent('main');
for (const n of ['Obat Harian','Obat Interval','Obat Mingguan','Obat Siklus','Obat PRN'])
  check(`"${n}" ada di daftar`, list?.includes(n));

// Cek di backend: kelima pola tersimpan benar
const tok=(await (await fetch(`${API}/auth/login`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email,password})})).json()).data.access_token;
const meds=(await (await fetch(`${API}/medications`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json()).data;
check('backend menyimpan 5 obat', meds?.length===5, `${meds?.length} obat`);
const detail=await (await fetch(`${API}/medications/${meds.find(m=>m.name==='Obat PRN').id}`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json();
check('PRN: as_needed true', detail.data?.as_needed===true);
check('PRN: tidak punya jam', (detail.data?.times??[]).length===0, `${(detail.data?.times??[]).length} jam`);
const wd=await (await fetch(`${API}/medications/${meds.find(m=>m.name==='Obat Mingguan').id}`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json();
check('mingguan: days_of_week = Senin+Kamis (9)', wd.data?.frequency?.days_of_week===9, `mask ${wd.data?.frequency?.days_of_week}`);
const iv=await (await fetch(`${API}/medications/${meds.find(m=>m.name==='Obat Interval').id}`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json();
check('interval: interval_hours=8', iv.data?.frequency?.interval_hours===8);
const cy=await (await fetch(`${API}/medications/${meds.find(m=>m.name==='Obat Siklus').id}`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json();
check('siklus: 21/7', cy.data?.frequency?.cycle_intake_days===21 && cy.data?.frequency?.cycle_pause_days===7);
const dl=await (await fetch(`${API}/medications/${meds.find(m=>m.name==='Obat Harian').id}`,{headers:{Authorization:`Bearer ${tok}`,Origin:ORIGIN}})).json();
check('harian: 2 jam', (dl.data?.times??[]).length===2);

// Hari Ini harus menampilkan dosis (4 obat terjadwal, PRN tidak)
await p.goto(BASE+'/hari-ini',{waitUntil:'networkidle'});
await p.waitForTimeout(1500);
const hari=await p.textContent('main');
check('Hari Ini menampilkan dosis dari pola non-PRN', (hari?.includes('Obat Harian')||hari?.includes('Obat Interval')));
check('PRN tidak muncul sebagai dosis terjadwal', !hari?.includes('Obat PRN'));

await p.screenshot({path:'/tmp/po-obat-list.png'});
await p.goto(BASE+'/obat',{waitUntil:'networkidle'}); await p.waitForTimeout(600);
await p.screenshot({path:'/tmp/po-obat-list.png'});
await p.locator('button:has-text("Tambah obat")').first().click(); await p.waitForTimeout(900);
await p.screenshot({path:'/tmp/po-wizard-open.png'});

console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
