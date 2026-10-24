package local.gemma.launcher;

import android.app.*;
import android.content.*;
import android.content.pm.ServiceInfo;
import android.os.*;
import org.json.JSONObject;
import java.io.*;
import java.net.*;
import java.security.*;
import java.util.*;
import java.util.concurrent.*;

/** 初回セットアップ中だけ、同梱物を認証付きloopbackでTermuxへ渡す。 */
public class SetupService extends Service {
    static volatile String base="", message="セットアップの準備中です", state="idle";
    static volatile String imageToken="";
    private ServerSocket server;
    private boolean bound;
    private final ServiceConnection connection=new ServiceConnection(){
        public void onServiceConnected(ComponentName n,IBinder b){}
        public void onServiceDisconnected(ComponentName n){}
    };
    private final ExecutorService io=Executors.newFixedThreadPool(4);
    @Override public void onCreate() {
        super.onCreate();
        NotificationManager nm=getSystemService(NotificationManager.class);
        nm.createNotificationChannel(new NotificationChannel("setup","Gemmaの初回準備",NotificationManager.IMPORTANCE_LOW));
        PendingIntent open=PendingIntent.getActivity(this,2,new Intent(this,SetupActivity.class),PendingIntent.FLAG_IMMUTABLE);
        Notification n=new Notification.Builder(this,"setup").setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle("Gemmaの初回セットアップ").setContentText("端末内のNPU環境を準備します").setContentIntent(open).setOngoing(true).build();
        if(Build.VERSION.SDK_INT>=34)startForeground(2,n,ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);else startForeground(2,n);
        try {
            byte[] random=new byte[24];new SecureRandom().nextBytes(random);
            StringBuilder token=new StringBuilder();for(byte b:random)token.append(String.format(Locale.ROOT,"%02x",b&255));
            server=new ServerSocket(0,8,InetAddress.getByName("127.0.0.1"));
            base="http://127.0.0.1:"+server.getLocalPort()+"/"+token;
            io.execute(()->{while(!server.isClosed())try{Socket client=server.accept();io.execute(()->serve(client));}catch(IOException ignored){break;}});
            message="Termuxで初回セットアップを開始できます";
        }catch(IOException e){state="error";message="準備用のローカル接続を開始できません";}
    }
    private static String line(InputStream in)throws IOException {
        ByteArrayOutputStream b=new ByteArrayOutputStream();int c;
        while((c=in.read())!=-1&&c!='\n'){if(b.size()>4096)throw new IOException("Header too large");if(c!='\r')b.write(c);}
        return b.toString("US-ASCII");
    }
    private void reply(OutputStream out,int code,String type,byte[] data)throws IOException {
        out.write(("HTTP/1.1 "+code+"\r\nContent-Type: "+type+"\r\nContent-Length: "+data.length+"\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n").getBytes("US-ASCII"));out.write(data);out.flush();
    }
    private byte[] asset(String name)throws IOException {
        try(InputStream in=getAssets().open(name);ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[8192];int n;while((n=in.read(b))>0)out.write(b,0,n);return out.toByteArray();}
    }
    private void serve(Socket client) {
        try(Socket s=client){
            s.setSoTimeout(15000);InputStream in=new BufferedInputStream(s.getInputStream());OutputStream out=s.getOutputStream();
            String[] request=line(in).split(" ");if(request.length!=3)return;
            Map<String,String> headers=new HashMap<>();String h;int count=0;
            while(!(h=line(in)).isEmpty()){if(++count>40)return;int p=h.indexOf(':');if(p>0)headers.put(h.substring(0,p).toLowerCase(Locale.ROOT),h.substring(p+1).trim());}
            URI uri=URI.create(base);
            if(!uri.getRawAuthority().equals(headers.get("host"))||headers.containsKey("origin")||!request[1].startsWith(uri.getPath()+"/")){
                reply(out,403,"text/plain",new byte[0]);return;
            }
            String path=request[1].substring(uri.getPath().length());
            if(request[0].equals("POST")&&path.equals("/progress")){
                int length=Integer.parseInt(headers.getOrDefault("content-length","0"));if(length<1||length>8192)return;
                byte[] body=new byte[length];new DataInputStream(in).readFully(body);
                JSONObject progress=new JSONObject(new String(body,"UTF-8"));
                String next=progress.optString("state");if(!Arrays.asList("running","complete","error").contains(next))return;
                message=progress.getString("message");state=next;
                if(next.equals("complete"))getSharedPreferences("setup",0).edit().putBoolean("installed",true).apply();
                reply(out,200,"application/json","{}".getBytes("UTF-8"));return;
            }
            if(!request[0].equals("GET")){reply(out,405,"text/plain",new byte[0]);return;}
            if(path.equals("/bootstrap.sh")){
                String sha=new String(asset("runtime.sha256"),"US-ASCII").trim();
                String script="export SETUP_URL='"+base+"'\nexport RUNTIME_SHA='"+sha+"'\n"+new String(asset("bootstrap.sh"),"UTF-8");
                reply(out,200,"text/plain",script.getBytes("UTF-8"));
            }else if(path.equals("/runtime.tar")){
                // APK assetsは同梱時のhashでTermux側でも照合する。
                android.content.res.AssetFileDescriptor fd=getAssets().openFd("runtime.tar");
                out.write(("HTTP/1.1 200 OK\r\nContent-Length: "+fd.getLength()+"\r\nContent-Type: application/octet-stream\r\nConnection: close\r\n\r\n").getBytes("US-ASCII"));
                try(InputStream data=fd.createInputStream()){byte[] b=new byte[65536];int n;while((n=data.read(b))>0)out.write(b,0,n);}finally{fd.close();}
            }else if(path.equals("/image-token")&&!imageToken.isEmpty())reply(out,200,"text/plain",imageToken.getBytes("US-ASCII"));
            else reply(out,404,"text/plain",new byte[0]);
        }catch(Exception ignored){/* 接続の秘密値や設定トークンはログへ出さない。 */}
    }
    @Override public int onStartCommand(Intent i,int f,int id){
        if(!bound)try{bound=bindService(new Intent().setClassName("com.termux","com.termux.app.RunCommandService"),connection,Context.BIND_AUTO_CREATE|Context.BIND_IMPORTANT);}catch(RuntimeException ignored){}
        return START_NOT_STICKY;
    }
    @Override public IBinder onBind(Intent i){return null;}
    @Override public void onDestroy(){base="";imageToken="";if(bound)unbindService(connection);try{if(server!=null)server.close();}catch(IOException ignored){}io.shutdownNow();super.onDestroy();}
}
