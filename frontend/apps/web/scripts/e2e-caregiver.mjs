import { chromium } from 'playwright';
const API='http://127.0.0.1:8081/api/v1', BASE='http://127.0.0.1:5173', ORIGIN='http://localhost:5173';
const password='SandiUji123!';
let fail=0; const check=(l,ok,d='')=>{console.log(`${ok?'OK  ':'GAGAL'} ${l}${d?' — '+d:''}`);if(!ok)fail++;};

// Dua akun: pemilik (owner) dan pendamping (caregiver)
const ownerEmail=`own-${Date.now()}@example.com`, cgEmail=`cg-${Date.now()}@example.com`;
const reg=async(e,n)=>(await (await fetch(`${API}/auth/register`,{method:'POST',headers:{'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({email:e,password,full_name:n,timezone:'Asia/Jakarta'})})).json()).data.access_token;
const ownerTok=await reg(ownerEmail,'Pemilik Data');
const cgTok=await reg(cgEmail,'Pendamping');

// Owner undang caregiver lewat API (agar token undangan bisa diambil untuk uji terima)
const inv=await fetch(`${API}/care-links`,{method:'POST',headers:{Authorization:`Bearer ${ownerTok}`,'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({invite_email:cgEmail,relation:'child',role:'helper'})});
const invBody=await inv.json();
check('undangan dibuat', inv.status===201||inv.status===200, `status ${inv.status}`);
// Token undangan ada di balasan?
const inviteUrl = invBody.data?.invite_url;
check('balasan undangan memuat invite_url (sekali saja)', !!inviteUrl);
const inviteToken = inviteUrl ? new URL(inviteUrl).searchParams.get('token') : null;

const b=await chromium.launch({executablePath:process.env.CHROME_PATH});
const p=await (await b.newContext({ viewport: {width:390,height:844}, locale: 'id-ID',deviceScaleFactor:2})).newPage();
const errs=[]; p.on('pageerror',e=>errs.push(e.message));

// Login sebagai OWNER, lihat halaman caregiver
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',ownerEmail); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.goto(BASE+'/caregiver',{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
let t=await p.textContent('main');
check('halaman caregiver memuat', (await p.textContent('h1'))?.includes('Caregiver'));
check('undangan tampil di daftar', t?.includes(cgEmail), '');
check('peran helper tampil', t?.includes('Membantu mencatat'));
check('status Menunggu tampil', t?.includes('Menunggu'));
check('tombol Cabut akses ada', (await p.locator('button:has-text("Cabut akses")').count())>0);

// Undang lewat UI
await p.locator('button:has-text("Undang")').first().click(); await p.waitForTimeout(700);
check('dialog undang terbuka', (await p.textContent('body'))?.includes('Hubungan'));
await p.fill('#i-email',`baru-${Date.now()}@example.com`);
await p.locator('button:has-text("Kirim undangan")').click(); await p.waitForTimeout(2200);
t=await p.textContent('main');
check('undangan kedua tampil', (t?.match(/Menunggu/g)??[]).length>=2, `${(t?.match(/Menunggu/g)??[]).length} menunggu`);

// Cabut akses yang pertama
await p.locator('button:has-text("Cabut akses")').first().click(); await p.waitForTimeout(2200);
t=await p.textContent('main');
check('setelah cabut: ada status Dicabut', t?.includes('Dicabut'), '');

// Login sebagai CAREGIVER.
// CATATAN PENTING: undangan yang masih `pending` BELUM terhubung ke akun
// caregiver — `caregiver_id` baru terisi saat undangan diterima. Jadi wajar
// bila daftar "siapa yang saya pantau" masih kosong pada tahap ini. Itu
// perilaku backend yang benar, bukan halaman yang rusak.
await p.goto(BASE+'/login',{waitUntil:'networkidle'});
await p.fill('#email',cgEmail); await p.fill('#password',password);
await p.click('button[type="submit"]'); await p.waitForURL(/hari-ini/,{timeout:20000}).catch(()=>{});
await p.goto(BASE+'/caregiver',{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
const t2=await p.textContent('main');
check('sebelum terima: belum ada relasi aktif', !t2?.includes('Aktif'), '');

// Terima undangan lewat token (alur sebenarnya: tautan dari email)
if (inviteToken) {
  const acc=await fetch(`${API}/care-links/accept`,{method:'POST',headers:{Authorization:`Bearer ${cgTok}`,'Content-Type':'application/json',Origin:ORIGIN},body:JSON.stringify({token:inviteToken})});
  check('terima undangan lewat token', acc.status===200, `status ${acc.status}`);
  await p.reload({waitUntil:'networkidle'}); await p.waitForTimeout(1500);
  const t3=await p.textContent('main');
  check('setelah terima: caregiver melihat relasinya', t3?.includes('Undangan untuk Anda'), '');
  check('status Aktif tampil', t3?.includes('Aktif'), '');
  check('peran helper tampil ke caregiver', t3?.includes('Membantu mencatat'), '');
}

await p.screenshot({path:'/tmp/po-caregiver.png'});
console.log('\n=== error ==='); console.log(errs.length?errs.join('\n'):'(tidak ada)');
console.log(fail===0?'\nSEMUA LULUS':`\n${fail} GAGAL`);
await b.close();
process.exit(fail===0?0:1);
