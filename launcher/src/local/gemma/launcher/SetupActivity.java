package local.gemma.launcher;

import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.*;
import android.provider.Settings;
import android.text.InputType;
import android.widget.*;
import java.io.*;
import java.net.*;
import java.security.*;
import java.util.*;
import java.util.concurrent.*;

public class SetupActivity extends Activity {
    private TextView status;
    private final Handler main=new Handler(Looper.getMainLooper());
    private final ExecutorService worker=Executors.newSingleThreadExecutor();
    private volatile boolean downloading;
    private final Runnable refresh=new Runnable(){public void run(){if(isDestroyed())return;if(!downloading)status.setText(SetupService.message);main.postDelayed(this,1000);}};
    private boolean supported(){
        ActivityManager.MemoryInfo m=new ActivityManager.MemoryInfo();getSystemService(ActivityManager.class).getMemoryInfo(m);
        return Build.VERSION.SDK_INT>=31&&Build.SOC_MODEL.toUpperCase(Locale.ROOT).startsWith("SM8650")&&m.totalMem>=14L*1024*1024*1024;
    }
    private boolean installed(String p){try{getPackageManager().getPackageInfo(p,0);return true;}catch(PackageManager.NameNotFoundException e){return false;}}
    private void note(String text){new AlertDialog.Builder(this).setMessage(text).setPositiveButton("OK",null).show();}
    private void button(LinearLayout parent,String text,Runnable action){Button b=new Button(this);b.setText(text);b.setOnClickListener(v->action.run());parent.addView(b);}
    @Override public void onCreate(Bundle saved){
        super.onCreate(saved);
        ScrollView scroll=new ScrollView(this);LinearLayout list=new LinearLayout(this);list.setOrientation(LinearLayout.VERTICAL);list.setPadding(32,48,32,48);scroll.addView(list);setContentView(scroll);
        getWindow().getDecorView().setSystemUiVisibility(android.view.View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR);
        scroll.setOnApplyWindowInsetsListener((v,insets)->{v.setPadding(0,insets.getSystemWindowInsetTop(),0,insets.getSystemWindowInsetBottom());return insets;});
        TextView title=new TextView(this);title.setText("Gemma 初回セットアップ");title.setTextSize(26);list.addView(title);
        TextView device=new TextView(this);device.setText("端末: "+Build.MODEL+" / "+(Build.VERSION.SDK_INT>=31?Build.SOC_MODEL:Build.HARDWARE)+"\n対象: Snapdragon 8 Gen 3・RAM 16GB\nBolt取得は約6.2GB、初回は空き16GiB以上が必要です。Wi-Fiを利用してください。\nモデルの処理は端末のNPUで行います。");list.addView(device);
        status=new TextView(this);status.setPadding(0,24,0,24);list.addView(status);
        if(!supported()){status.setText("この端末に対応するNPU版は未提供です。CPUへ切り替えず、ここで停止します。");return;}
        startForegroundService(new Intent(this,SetupService.class));main.post(refresh);
        if(Build.VERSION.SDK_INT>=33&&checkSelfPermission("android.permission.POST_NOTIFICATIONS")!=PackageManager.PERMISSION_GRANTED)requestPermissions(new String[]{"android.permission.POST_NOTIFICATIONS"},2);
        button(list,"1. Termuxをインストール / 開く",()->installApp("termux"));
        button(list,"2. セットアップをコピーしてTermuxを開く",this::bootstrap);
        TextView explain=new TextView(this);explain.setText("初回のみ、Termuxの準備が終わったら入力欄を長押し → 貼り付け → Enter。ダウンロード・PLE分離・NPU確認が終わったらGemmaへ戻ってください。再試行時は途中から再開します。");list.addView(explain);
        button(list,"3. 会話を開く",()->startActivity(new Intent(this,MainActivity.class)));
        button(list,"ホーム画面にGemmaを追加",()->startActivity(new Intent(this,MainActivity.class).putExtra("pin_to_home",true)));
        button(list,"画像生成: Nightmareをインストール / 開く",()->installApp("nightmare"));
        TextView imageInfo=new TextView(this);imageInfo.setText("画像生成を使う場合はNightmare側でNPU対応モデルを導入し、Settings → Advanced → APIを有効にしてください。表示されるトークンを下に貼り付けると接続できます。設定済みの端末では入力不要です。");list.addView(imageInfo);
        EditText token=new EditText(this);token.setHint("NightmareのAPIトークン");token.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);list.addView(token);
        button(list,"画像エンジンへ接続",()->connectImages(token.getText().toString().trim()));
    }
    private void bootstrap(){
        if(!installed("com.termux")){note("先にTermuxをインストールして、一度開いてください。");return;}
        if(checkSelfPermission("com.termux.permission.RUN_COMMAND")!=PackageManager.PERMISSION_GRANTED){requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"},3);note("権限を許可してから、もう一度セットアップを押してください。");return;}
        if(SetupService.base.isEmpty()){note("準備中です。数秒後に押してください。");return;}
        startForegroundService(new Intent(this,SetupService.class));
        String command="curl -fsS --connect-timeout 10 '"+SetupService.base+"/bootstrap.sh' -o \"$HOME/.gemma-setup.sh\" && bash \"$HOME/.gemma-setup.sh\"";
        if(getSharedPreferences("setup",0).getBoolean("installed",false)){
            Intent run=new Intent("com.termux.RUN_COMMAND").setClassName("com.termux","com.termux.app.RunCommandService");
            run.putExtra("com.termux.RUN_COMMAND_PATH","/data/data/com.termux/files/usr/bin/sh");
            run.putExtra("com.termux.RUN_COMMAND_ARGUMENTS",new String[]{"-c",command});run.putExtra("com.termux.RUN_COMMAND_BACKGROUND",true);
            try{startService(run);SetupService.message="セットアップを再確認しています。既存モデルは再取得しません。";return;}catch(RuntimeException ignored){}
        }
        getSystemService(ClipboardManager.class).setPrimaryClip(ClipData.newPlainText("Gemmaセットアップ",command));
        startActivity(getPackageManager().getLaunchIntentForPackage("com.termux"));
    }
    private void connectImages(String token){
        if(!token.matches("[a-fA-F0-9]{48}")){note("NightmareのAPI画面に表示される48文字のトークンを入力してください。");return;}
        if(checkSelfPermission("com.termux.permission.RUN_COMMAND")!=PackageManager.PERMISSION_GRANTED){requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"},3);note("権限を許可してから、もう一度接続を押してください。");return;}
        SetupService.imageToken=token;
        String root="/data/data/com.termux/files/home/gemma-e4b";
        String command="umask 077; curl -fsS '"+SetupService.base+"/image-token' -o '"+root+"/nightmare.token.new' && mv '"+root+"/nightmare.token.new' '"+root+"/nightmare.token' && sh '"+root+"/restart-after-setup.sh'";
        Intent run=new Intent("com.termux.RUN_COMMAND").setClassName("com.termux","com.termux.app.RunCommandService");
        run.putExtra("com.termux.RUN_COMMAND_PATH","/data/data/com.termux/files/usr/bin/sh");
        run.putExtra("com.termux.RUN_COMMAND_ARGUMENTS",new String[]{"-c",command});run.putExtra("com.termux.RUN_COMMAND_BACKGROUND",true);
        try{startService(run);note("接続設定を渡しました。Gemmaの会話画面で接続状態を確認してください。");}catch(RuntimeException e){note("Termuxの初回セットアップを先に完了してください。");}
    }
    private static String sha(File f)throws Exception {MessageDigest d=MessageDigest.getInstance("SHA-256");try(InputStream in=new FileInputStream(f)){byte[] b=new byte[65536];int n;while((n=in.read(b))>0)d.update(b,0,n);}StringBuilder s=new StringBuilder();for(byte b:d.digest())s.append(String.format(Locale.ROOT,"%02x",b&255));return s.toString();}
    private void installApp(String app){
        String pkg=app.equals("termux")?"com.termux":"com.abrah.nightmare";
        if(installed(pkg)){startActivity(getPackageManager().getLaunchIntentForPackage(pkg));return;}
        if(!getPackageManager().canRequestPackageInstalls()){
            note("Gemmaから公式APKをインストールするため、次の画面で「この提供元を許可」をオンにし、戻って同じボタンを押してください。");
            startActivity(new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,Uri.parse("package:"+getPackageName())));return;
        }
        if(downloading)return;downloading=true;
        String url=app.equals("termux")?"https://github.com/termux/termux-app/releases/download/v0.118.3/termux-app_v0.118.3%2Bgithub-debug_arm64-v8a.apk":"https://github.com/AbrahamPaulJ/nightmare-mobile/releases/download/v1.6.211/nightmare-1.6.211.apk";
        String expected=app.equals("termux")?"72fdb596045116bf5ba1b5bdf5b26fddb9acc0bd074ad9f2da9eb0ae85e83a4e":"51d8366e1c644285d0f9d675170f11504b0623767d47b98a68216c068cbe2f1f";
        worker.execute(()->{
            try{
                File target=new File(getCacheDir(),app+".apk");
                if(!target.isFile()||!sha(target).equals(expected)){
                    HttpURLConnection c=(HttpURLConnection)new URL(url).openConnection();c.setConnectTimeout(20000);c.setReadTimeout(30000);
                    File part=new File(getCacheDir(),app+".apk.part");
                    try(InputStream in=c.getInputStream();OutputStream out=new FileOutputStream(part)){
                        byte[] b=new byte[65536];int n;long size=0,last=0;
                        while((n=in.read(b))>0){out.write(b,0,n);size+=n;if(size>100L*1024*1024)throw new IOException("APK size exceeds limit");if(size-last>1024*1024){last=size;String msg=app+"取得中: "+size/(1024*1024)+" MB";main.post(()->status.setText(msg));}}
                    }finally{c.disconnect();}
                    if(!sha(part).equals(expected))throw new IOException("APKのSHA256が一致しません");
                    if(!part.renameTo(target))throw new IOException("APKを保存できません");
                }
                main.post(()->{if(isDestroyed())return;Intent install=new Intent(Intent.ACTION_VIEW).setDataAndType(Uri.parse("content://local.gemma.launcher.install/"+app+".apk"),"application/vnd.android.package-archive").addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);startActivity(install);});
            }catch(Exception e){main.post(()->{if(!isDestroyed())note("取得・検証に失敗しました。通信を確認して再試行してください。 "+e.getMessage());});}
            finally{downloading=false;}
        });
    }
    @Override public void onDestroy(){main.removeCallbacksAndMessages(null);worker.shutdownNow();super.onDestroy();}
}
