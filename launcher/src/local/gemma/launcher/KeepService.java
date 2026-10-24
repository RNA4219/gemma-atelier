package local.gemma.launcher;

import android.app.*;
import android.content.*;
import android.content.pm.ServiceInfo;
import android.os.*;
import org.json.JSONObject;
import java.net.*;
import java.io.*;
import java.util.concurrent.*;

public class KeepService extends Service {
    static volatile String status = "Boltを起動しています…";
    static volatile boolean ready;
    private final ScheduledExecutorService worker = Executors.newSingleThreadScheduledExecutor();
    private volatile boolean bound;
    private long lastStart;
    private final ServiceConnection connection = new ServiceConnection() {
        public void onServiceConnected(ComponentName name, IBinder binder) { status = "Boltの準備を待っています…"; }
        public void onServiceDisconnected(ComponentName name) { ready = false; }
        public void onBindingDied(ComponentName name) { unbind(); bindTermux(); }
    };

    @Override public void onCreate() {
        super.onCreate();
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel("running", "Gemmaの起動と接続", NotificationManager.IMPORTANCE_LOW));
        PendingIntent open = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class), PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT);
        Notification notification = new Notification.Builder(this, "running").setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle("Gemma · Bolt").setContentText("端末内の会話・画像生成へ接続しています")
            .setOngoing(true).setContentIntent(open).build();
        if (Build.VERSION.SDK_INT >= 34) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        else startForeground(1, notification);
        bindTermux();
        worker.scheduleWithFixedDelay(this::check, 0, 20, TimeUnit.SECONDS);
    }

    private void bindTermux() {
        try {
            Intent service = new Intent().setClassName("com.termux", "com.termux.app.RunCommandService");
            bound = bindService(service, connection, Context.BIND_AUTO_CREATE|Context.BIND_IMPORTANT);
            if (!bound) status = "Termuxへの接続を開始できませんでした";
        } catch (RuntimeException error) { status = "Termuxの実行権限を確認してください"; }
    }

    private void unbind() { if (bound) { bound = false; unbindService(connection); } }

    static boolean health() {
        HttpURLConnection connection = null;
        try {
            connection = (HttpURLConnection)new URL("http://127.0.0.1:8085/api/status").openConnection();
            connection.setConnectTimeout(1500); connection.setReadTimeout(4500);
            try (InputStream input = connection.getInputStream(); ByteArrayOutputStream data = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[4096]; int count;
                while ((count=input.read(buffer))!=-1) { data.write(buffer,0,count); if(data.size()>65536)return false; }
                JSONObject result = new JSONObject(data.toString("UTF-8"));
                return result.optBoolean("gemma");
            }
        } catch (Exception error) { return false; }
        finally { if(connection!=null)connection.disconnect(); }
    }

    private void check() {
        ready = health();
        if (ready) { status = "準備できました"; return; }
        long now = SystemClock.elapsedRealtime();
        if (lastStart != 0 && now-lastStart < 30000) return;
        lastStart = now;
        try {
            Intent command = new Intent("com.termux.RUN_COMMAND").setClassName("com.termux", "com.termux.app.RunCommandService");
            command.putExtra("com.termux.RUN_COMMAND_PATH", "/data/data/com.termux/files/usr/bin/sh");
            command.putExtra("com.termux.RUN_COMMAND_ARGUMENTS", new String[]{"/data/data/com.termux/files/home/gemma-e4b/start-from-home.sh"});
            command.putExtra("com.termux.RUN_COMMAND_BACKGROUND", true);
            command.putExtra("com.termux.RUN_COMMAND_COMMAND_LABEL", "Gemma · Bolt起動");
            startService(command);
            status = "Boltを起動しています。しばらくお待ちください…";
        } catch (RuntimeException error) { status = "Termuxを起動できませんでした。権限設定を確認してください"; }
    }

    @Override public int onStartCommand(Intent intent,int flags,int id) { worker.execute(this::check); return START_STICKY; }
    @Override public IBinder onBind(Intent intent) { return null; }
    @Override public void onDestroy() { worker.shutdownNow(); unbind(); super.onDestroy(); }
}
