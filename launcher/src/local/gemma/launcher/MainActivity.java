package local.gemma.launcher;

import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.content.pm.ShortcutInfo;
import android.content.pm.ShortcutManager;
import android.graphics.drawable.Icon;
import android.net.Uri;
import android.os.*;
import android.view.*;
import android.widget.*;
import java.util.concurrent.*;

public class MainActivity extends Activity {
    private final Handler main = new Handler(Looper.getMainLooper());
    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private TextView status;
    private boolean opened;
    private long deadline;
    private final Runnable probe = new Runnable() {
        public void run() {
            if (isFinishing() || isDestroyed() || opened) return;
            status.setText(KeepService.status);
            worker.execute(() -> {
                boolean ready = KeepService.health();
                main.post(() -> {
                    if (isFinishing() || isDestroyed() || opened) return;
                    if (ready) openBrowser();
                    else if (SystemClock.elapsedRealtime() < deadline) main.postDelayed(probe, 1500);
                    else status.setText("起動に時間がかかっています。「もう一度接続」で再試行できます。");
                });
            });
        }
    };

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        ShortcutBranding.refresh(this);
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL); layout.setGravity(Gravity.CENTER); layout.setPadding(32,48,32,48);
        layout.setBackgroundColor(0xff14211e);
        TextView title = new TextView(this); title.setText(ShortcutBranding.NAME); title.setTextSize(32); title.setTextColor(0xffc9ef9b); layout.addView(title);
        status = new TextView(this); status.setTextColor(0xffeeeeee); status.setTextSize(17); status.setGravity(Gravity.CENTER); status.setPadding(0,32,0,32); layout.addView(status);
        Button retry = new Button(this); retry.setText("もう一度接続"); retry.setOnClickListener(v -> begin()); layout.addView(retry);
        Button setup = new Button(this); setup.setText("モデル・ダウンロード"); setup.setOnClickListener(v -> startActivity(new Intent(this,SetupActivity.class))); layout.addView(setup);
        setContentView(layout);
        if (getIntent().getBooleanExtra("pin_to_home", false)) { pinToHome(); return; }
        try { getPackageManager().getPackageInfo("com.termux",0); }
        catch (PackageManager.NameNotFoundException missing) { startActivity(new Intent(this,SetupActivity.class)); return; }
        if(!getSharedPreferences("setup",0).getBoolean("installed",false)){startActivity(new Intent(this,SetupActivity.class));return;}
        if (checkSelfPermission("com.termux.permission.RUN_COMMAND") != PackageManager.PERMISSION_GRANTED)
            requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"}, 1);
        else begin();
    }

    private void begin() {
        if(getSharedPreferences("setup",0).getBoolean("installed",false)&&!SetupService.runtimeCurrent(this)){
            startActivity(new Intent(this,SetupActivity.class).putExtra("update_and_open",true));finish();return;
        }
        opened=false; deadline=SystemClock.elapsedRealtime()+180000;
        main.removeCallbacks(probe);
        try { startForegroundService(new Intent(this, KeepService.class)); main.post(probe); }
        catch (RuntimeException error) { status.setText("起動サービスを開始できませんでした"); }
    }

    @Override public void onNewIntent(Intent intent) {
        super.onNewIntent(intent); setIntent(intent);
        if (intent.getBooleanExtra("pin_to_home", false)) pinToHome();
        else if (checkSelfPermission("com.termux.permission.RUN_COMMAND") != PackageManager.PERMISSION_GRANTED)
            requestPermissions(new String[]{"com.termux.permission.RUN_COMMAND"}, 1);
        else begin();
    }

    private void pinToHome() {
        ShortcutManager shortcuts = getSystemService(ShortcutManager.class);
        if (shortcuts.isRequestPinShortcutSupported()) {
            ShortcutInfo shortcut = ShortcutBranding.shortcut(this);
            shortcuts.requestPinShortcut(shortcut, null);
            status.setText("ホーム画面への追加を確定してください。次回からLLM Atelierを押すだけでBoltを起動できます。");
        } else status.setText("アプリ一覧のLLM Atelierをホーム画面へ追加してください。");
    }
    @Override public void onRequestPermissionsResult(int code,String[] permissions,int[] results) {
        super.onRequestPermissionsResult(code,permissions,results);
        if(code==1 && results.length>0 && results[0]==PackageManager.PERMISSION_GRANTED)begin();
        else status.setText("Boltの起動にはTermuxの実行許可が必要です");
    }

    private void openBrowser() {
        opened=true;
        if (getSharedPreferences("setup",0).getBoolean("installed",false)&&!DownloadState.busy(this)) stopService(new Intent(this,SetupService.class));
        Intent page = new Intent(Intent.ACTION_VIEW,Uri.parse("http://127.0.0.1:8085/"));
        page.setPackage("com.android.chrome"); page.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try { startActivity(page); finish(); }
        catch (RuntimeException error) { opened=false; status.setText("Chromeで画面を開けませんでした"); }
    }
    @Override public void onDestroy() { main.removeCallbacksAndMessages(null); worker.shutdownNow(); super.onDestroy(); }
}
