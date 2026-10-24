package local.gemma.launcher;

import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.*;
import android.provider.Settings;
import android.webkit.*;
import org.json.*;
import java.io.*;
import java.net.*;
import java.util.*;
import java.util.concurrent.*;

/** 同梱したUIだけに、列挙した導入操作を公開する。 */
public class SetupActivity extends Activity {
    private WebView web;
    private final Handler main=new Handler(Looper.getMainLooper());
    private final ExecutorService worker=Executors.newSingleThreadExecutor();
    private volatile JSONObject health=new JSONObject();
    private String pendingPackage="",pendingAction="",pendingToken="";
    private boolean launching,resumed;
    private final Runnable advance=new Runnable(){public void run(){
        if(isDestroyed())return;
        if(resumed&&DownloadState.prefs(SetupActivity.this).getBoolean("install_flow",false)){
            String stage=DownloadState.prefs(SetupActivity.this).getString("flow_stage","");
            if(stage.equals("download_termux")&&DownloadState.job(SetupActivity.this,"termux").optString("state").equals("ready")){
                DownloadState.prefs(SetupActivity.this).edit().putString("flow_stage","install_termux").apply();installApp("termux");
            }
        }main.postDelayed(this,1500);
    }};
    private final Runnable finishUpdate=new Runnable(){public void run(){
        if(isDestroyed()||isFinishing())return;
        if(SetupService.runtimeCurrent(SetupActivity.this)){
            startActivity(new Intent(SetupActivity.this,MainActivity.class));finish();
        }else main.postDelayed(this,1000);
    }};
    private final Runnable probe=new Runnable(){public void run(){
        if(isDestroyed())return;
        worker.execute(()->{
            JSONObject next=new JSONObject();
            try{
                HttpURLConnection c=(HttpURLConnection)new URL("http://127.0.0.1:8085/api/status").openConnection();c.setConnectTimeout(1000);c.setReadTimeout(1000);
                try(InputStream in=c.getInputStream();ByteArrayOutputStream out=new ByteArrayOutputStream()){
                    byte[] b=new byte[2048];int n;while((n=in.read(b))>0){if(out.size()>32768)break;out.write(b,0,n);}next=new JSONObject(out.toString("UTF-8"));
                }finally{c.disconnect();}
            }catch(Exception ignored){}
            health=next;
        });main.postDelayed(this,5000);
    }};
    private boolean installed(String p){try{getPackageManager().getPackageInfo(p,0);return true;}catch(PackageManager.NameNotFoundException e){return false;}}
    private boolean supported(){
        ActivityManager.MemoryInfo m=new ActivityManager.MemoryInfo();getSystemService(ActivityManager.class).getMemoryInfo(m);
        return Build.VERSION.SDK_INT>=31&&Build.SOC_MODEL.toUpperCase(Locale.ROOT).startsWith("SM8650")&&m.totalMem>=14L*1024*1024*1024;
    }
    private void note(String text){new AlertDialog.Builder(this).setMessage(text).setPositiveButton("OK",null).show();}
    @Override public void onCreate(Bundle saved){
        super.onCreate(saved);ShortcutBranding.refresh(this);getWindow().setStatusBarColor(0xfff5f5f1);getWindow().setNavigationBarColor(0xfff5f5f1);
        getWindow().getDecorView().setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR|android.view.View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR);
        android.widget.FrameLayout frame=new android.widget.FrameLayout(this);
        web=new WebView(this);web.setBackgroundColor(0xfff5f5f1);frame.addView(web,new android.widget.FrameLayout.LayoutParams(-1,-1));setContentView(frame);
        frame.setOnApplyWindowInsetsListener((v,insets)->{v.setPadding(0,insets.getSystemWindowInsetTop(),0,insets.getSystemWindowInsetBottom());return insets.consumeSystemWindowInsets();});
        WebSettings settings=web.getSettings();settings.setJavaScriptEnabled(true);settings.setAllowFileAccess(false);settings.setAllowContentAccess(false);settings.setDomStorageEnabled(false);settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        web.setWebViewClient(new WebViewClient(){
            @Override public void onPageFinished(WebView v,String url){if(getIntent().getData()!=null&&"models".equals(getIntent().getData().getHost()))v.evaluateJavascript("tab('models')",null);}
            @Override public boolean shouldOverrideUrlLoading(WebView v,WebResourceRequest r){return true;}
            @Override public WebResourceResponse shouldInterceptRequest(WebView v,WebResourceRequest r){
                if(r.getUrl().toString().matches("file:///android_asset/(setup\\.(html|css|js)|setup-state\\.js)"))return null;
                return new WebResourceResponse("text/plain","UTF-8",new ByteArrayInputStream(new byte[0]));
            }
        });
        web.addJavascriptInterface(new Bridge(),"Atelier");web.loadUrl("file:///android_asset/setup.html");
        if(supported())startForegroundService(new Intent(this,SetupService.class));
        if(Build.VERSION.SDK_INT>=33&&checkSelfPermission("android.permission.POST_NOTIFICATIONS")!=PackageManager.PERMISSION_GRANTED)requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"},2);
        main.post(probe);main.post(advance);
        boolean updating=getIntent().getBooleanExtra("update_and_open",false)&&supported()&&DownloadState.prefs(this).getBoolean("installed",false);
        if(updating){main.postDelayed(this::bootstrap,900);main.postDelayed(finishUpdate,1000);}
        else if(DownloadState.job(this,"bolt").optString("state").equals("running")&&DownloadState.prefs(this).getBoolean("command_ready",false))main.postDelayed(this::bootstrap,900);
    }
    final class Bridge {
        @JavascriptInterface public String getState(){
            JSONObject s=new JSONObject();try{
                ActivityManager.MemoryInfo m=new ActivityManager.MemoryInfo();getSystemService(ActivityManager.class).getMemoryInfo(m);
                s.put("device",Build.MODEL).put("soc",Build.VERSION.SDK_INT>=31?Build.SOC_MODEL:Build.HARDWARE).put("ram",m.totalMem).put("free",getFilesDir().getUsableSpace())
                    .put("supported",supported()).put("termux",installed("com.termux")).put("nightmare",installed("com.abrah.nightmare"))
                    .put("installed",DownloadState.prefs(SetupActivity.this).getBoolean("installed",false)).put("commandReady",DownloadState.prefs(SetupActivity.this).getBoolean("command_ready",false))
                    .put("linked",DownloadState.prefs(SetupActivity.this).getBoolean("termux_linked",false)||DownloadState.prefs(SetupActivity.this).getBoolean("command_ready",false))
                    .put("termuxOpened",DownloadState.prefs(SetupActivity.this).getBoolean("termux_opened",false)).put("runtimeCurrent",SetupService.runtimeCurrent(SetupActivity.this))
                    .put("flowStage",DownloadState.prefs(SetupActivity.this).getString("flow_stage",""))
                    .put("runPermission",checkSelfPermission("com.termux.permission.RUN_COMMAND")==PackageManager.PERMISSION_GRANTED)
                    .put("wifi",DownloadState.wifi(SetupActivity.this)).put("wifiOnly",DownloadState.prefs(SetupActivity.this).getBoolean("wifi_only",true))
                    .put("origin",DownloadState.prefs(SetupActivity.this).getString("origin","https://huggingface.co")).put("health",health);
                JSONObject jobs=new JSONObject();for(String id:new String[]{"bolt","termux","nightmare"})jobs.put(id,DownloadState.job(SetupActivity.this,id));s.put("jobs",jobs);
            }catch(JSONException ignored){}return s.toString();
        }
        @JavascriptInterface public void action(String name,String data){
            if(data==null||data.length()>2048)return;
            main.post(()->{if(isDestroyed())return;try{act(name,new JSONObject(data));}catch(Exception e){note("操作を完了できませんでした。設定と通信を確認してください。");}});
        }
    }
    private void act(String name,JSONObject data)throws Exception{
        if(name.equals("preferences")){
            String origin=data.optString("origin","https://huggingface.co").trim();URI uri=new URI(origin);
            if(!"https".equals(uri.getScheme())||uri.getHost()==null||uri.getUserInfo()!=null||uri.getQuery()!=null||uri.getFragment()!=null||!(uri.getPath().isEmpty()||uri.getPath().equals("/"))){note("取得元は https://example.com の形式で入力してください。");return;}
            DownloadState.prefs(this).edit().putString("origin",origin).putBoolean("wifi_only",data.optBoolean("wifiOnly",true)).apply();web.evaluateJavascript("toast('ダウンロード設定を保存しました')",null);return;
        }
        if(name.equals("openChat")){startActivity(new Intent(this,MainActivity.class));return;}
        if(name.equals("pin")){startActivity(new Intent(this,MainActivity.class).putExtra("pin_to_home",true));return;}
        if(!supported()){note("この端末向けのNPU版は未提供です。");return;}
        if(name.equals("prepare")){prepare();return;}
        if(name.equals("repairLink")){linkTermux();return;}
        if(name.equals("openTermux")){openTermux();return;}
        if(name.equals("connectImages")){connectImages(data.optString("token").trim());return;}
        String id=data.optString("id","bolt");if(!Arrays.asList("bolt","termux","nightmare").contains(id))return;
        if(name.equals("pause")){
            String phase=DownloadState.job(this,id).optString("phase");if(!phase.equals("download")&&!phase.equals("waiting"))return;
            DownloadState.prefs(this).edit().putString("command_"+id,"pause").apply();web.evaluateJavascript("toast('一時停止しています。通信の応答をお待ちください')",null);return;
        }
        if(name.equals("reset")){
            if(DownloadState.job(this,id).optString("state").equals("running")){note("一時停止が完了してから取り直してください。");return;}
            new AlertDialog.Builder(this).setTitle("途中のダウンロードを取り直す？").setMessage("この取得の途中データだけを消します。導入済みのモデル・会話履歴は保持します。")
                .setNegativeButton("戻る",null).setPositiveButton("取り直す",(d,w)->{
                    if(id.equals("bolt")){DownloadState.prefs(this).edit().putString("command_bolt","reset").apply();bootstrap();}
                    else{File f=new File(getCacheDir(),id+".apk.part");if(f.exists()&&!f.delete()){note("途中データを削除できませんでした。");return;}startPackage(id);}
                }).show();return;
        }
        if(name.equals("start")){
            DownloadState.prefs(this).edit().putString("command_"+id,"run").apply();if(id.equals("bolt"))bootstrap();else installApp(id);
        }
    }
    private void prepare(){
        DownloadState.prefs(this).edit().putBoolean("install_flow",true).apply();
        if(!installed("com.termux")){
            String stage=DownloadState.job(this,"termux").optString("state").equals("ready")?"install_termux":"download_termux";
            DownloadState.prefs(this).edit().putString("flow_stage",stage).apply();installApp("termux");return;
        }
        if(!DownloadState.prefs(this).getBoolean("command_ready",false)&&!DownloadState.prefs(this).getBoolean("termux_linked",false)){
            if(!DownloadState.prefs(this).getBoolean("termux_opened",false)){openTermux();return;}
            linkTermux();return;
        }
        if(DownloadState.job(this,"bolt").optString("error_code").equals("termux_link")){linkTermux();return;}
        DownloadState.prefs(this).edit().putString("command_bolt","run").apply();bootstrap();
    }
    private void openTermux(){
        Intent intent=getPackageManager().getLaunchIntentForPackage("com.termux");
        if(intent==null){note("先にTermuxをインストールしてください。");return;}
        new AlertDialog.Builder(this).setTitle("Termuxの初期準備")
            .setMessage("Termuxを開き、最初の展開が終わったらLLM Atelierへ戻ってください。文字やコマンドの入力は不要です。次に「Termuxとの接続を許可」を押します。")
            .setPositiveButton("Termuxを開く",(d,w)->{DownloadState.prefs(this).edit().putBoolean("termux_opened",true).putString("flow_stage","link_termux").apply();startActivity(intent);}).setNegativeButton("あとで",null).show();
    }
    private void linkTermux(){
        if(!installed("com.termux")){installApp("termux");return;}
        new AlertDialog.Builder(this).setTitle("Termuxとの接続を許可")
            .setMessage("次の画面で「Termux」のフォルダを選び、「このフォルダを使用」→「許可」を押してください。表示されない場合は左上のメニューからTermuxを選びます。\n\n既存設定をバックアップし、LLM Atelierからの起動を許可する設定を追加します。コマンドの貼り付けは不要です。")
            .setPositiveButton("フォルダを選ぶ",(d,w)->startActivityForResult(TermuxLink.picker(),41)).setNegativeButton("あとで",null).show();
    }
    @Override protected void onActivityResult(int code,int result,Intent data){
        super.onActivityResult(code,result,data);
        if(code!=41)return;
        if(result!=RESULT_OK||data==null){note("接続設定はまだ完了していません。「Termuxとの接続を許可」から再開できます。");return;}
        Uri tree=data.getData();
        worker.execute(()->{
            try{
                TermuxLink.configure(this,tree);
                DownloadState.prefs(this).edit().putBoolean("termux_linked",true).putString("flow_stage","model").apply();
                DownloadState.put(this,"bolt","error_code","");
                main.post(()->{if(!isDestroyed())prepare();});
            }catch(Exception error){main.post(()->{if(!isDestroyed())note(error instanceof IOException?error.getMessage():"Termuxの設定へアクセスできませんでした。Termuxを一度開き、フォルダの許可をやり直してください。");});}
        });
    }
    private void bootstrap(){
        if(launching)return;if(!installed("com.termux")){installApp("termux");return;}
        if(!DownloadState.prefs(this).getBoolean("command_ready",false)&&!DownloadState.prefs(this).getBoolean("termux_linked",false)){prepare();return;}
        if(checkSelfPermission("com.termux.permission.RUN_COMMAND")!=PackageManager.PERMISSION_GRANTED){pendingAction="bootstrap";requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"},3);return;}
        startForegroundService(new Intent(this,SetupService.class));if(SetupService.base.isEmpty()){main.postDelayed(this::bootstrap,600);return;}
        launching=true;main.postDelayed(()->launching=false,2000);
        String command="curl -fsS --connect-timeout 10 '"+SetupService.base+"/bootstrap.sh' -o \"$HOME/.gemma-setup.sh\" && bash \"$HOME/.gemma-setup.sh\"";
        DownloadState.put(this,"bolt","state","running","phase","connection","error_code","","message","Termuxへ接続し、準備を始めます");
        DownloadState.prefs(this).edit().putString("flow_stage","model").apply();
        try{runTermux(command,true);web.evaluateJavascript("tab('downloads')",null);}
        catch(RuntimeException error){DownloadState.put(this,"bolt","state","error","error_code","termux_link","message","Termuxを起動できませんでした。接続の許可を設定し直してください。");}
    }
    private void runTermux(String command){
        runTermux(command,false);
    }
    private void runTermux(String command,boolean bootstrap){
        Intent run=new Intent("com.termux.RUN_COMMAND").setClassName("com.termux","com.termux.app.RunCommandService");
        if(bootstrap){
            String request=UUID.randomUUID().toString();DownloadState.prefs(this).edit().putString("bootstrap_request",request).apply();
            Intent result=new Intent(this,CommandResultReceiver.class).setData(Uri.parse("atelier-result:"+request)).putExtra("request",request);
            run.putExtra("com.termux.RUN_COMMAND_PENDING_INTENT",PendingIntent.getBroadcast(this,0,result,PendingIntent.FLAG_ONE_SHOT|PendingIntent.FLAG_MUTABLE));
        }
        run.putExtra("com.termux.RUN_COMMAND_COMMAND_LABEL","LLM Atelierの準備");
        run.putExtra("com.termux.RUN_COMMAND_PATH","/data/data/com.termux/files/usr/bin/sh");run.putExtra("com.termux.RUN_COMMAND_ARGUMENTS",new String[]{"-c",command});run.putExtra("com.termux.RUN_COMMAND_BACKGROUND",true);startService(run);
    }
    private void connectImages(String token){
        if(!token.matches("[a-fA-F0-9]{48}")){note("NightmareのAPI画面に表示される48文字のトークンを入力してください。");return;}
        if(checkSelfPermission("com.termux.permission.RUN_COMMAND")!=PackageManager.PERMISSION_GRANTED){pendingAction="images";pendingToken=token;requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"},3);return;}
        if(SetupService.base.isEmpty()){note("準備中です。数秒後に再度接続してください。");return;}
        SetupService.imageToken=token;String root="/data/data/com.termux/files/home/gemma-e4b";
        runTermux("umask 077; curl -fsS '"+SetupService.base+"/image-token' -o '"+root+"/nightmare.token.new' && mv '"+root+"/nightmare.token.new' '"+root+"/nightmare.token' && sh '"+root+"/restart-after-setup.sh'");note("接続設定を渡しました。数秒後に接続状態へ反映されます。");
    }
    private void startPackage(String id){DownloadState.prefs(this).edit().putString("command_"+id,"run").apply();startForegroundService(new Intent(this,SetupService.class).putExtra("download",id));}
    private void installApp(String id){
        String pkg=id.equals("termux")?"com.termux":"com.abrah.nightmare";
        if(installed(pkg)){Intent intent=getPackageManager().getLaunchIntentForPackage(pkg);if(intent!=null)startActivity(intent);return;}
        File apk=new File(getCacheDir(),id+".apk");if(!apk.isFile()||!DownloadState.job(this,id).optString("state").equals("ready")){startPackage(id);return;}
        if(!getPackageManager().canRequestPackageInstalls()){
            pendingPackage=id;new AlertDialog.Builder(this).setTitle("公式APKのインストール許可").setMessage("次の画面で「この提供元を許可」をオンにして戻ってください。続けてAndroidのインストール確認を開きます。")
                .setPositiveButton("設定を開く",(d,w)->startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+getPackageName())))).setNegativeButton("あとで",(d,w)->pendingPackage="").show();return;
        }
        startActivity(new Intent(Intent.ACTION_VIEW).setDataAndType(Uri.parse("content://local.gemma.launcher.install/"+id+".apk"),"application/vnd.android.package-archive").addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION));
    }
    @Override protected void onResume(){super.onResume();resumed=true;if(!pendingPackage.isEmpty()&&getPackageManager().canRequestPackageInstalls()){String id=pendingPackage;pendingPackage="";installApp(id);}}
    @Override protected void onPause(){resumed=false;super.onPause();}
    @Override public void onRequestPermissionsResult(int code,String[] p,int[] r){
        super.onRequestPermissionsResult(code,p,r);if(code==3){String action=pendingAction;pendingAction="";if(r.length>0&&r[0]==PackageManager.PERMISSION_GRANTED){if(action.equals("images")){connectImages(pendingToken);pendingToken="";}else bootstrap();}else note("セットアップにはTermuxの実行許可が必要です。");}
    }
    @Override public void onDestroy(){main.removeCallbacksAndMessages(null);worker.shutdownNow();if(web!=null){web.removeJavascriptInterface("Atelier");web.destroy();}super.onDestroy();}
}
