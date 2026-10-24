package local.gemma.launcher;

import android.content.Context;
import java.io.*;
import java.net.*;
import java.security.*;
import java.util.Locale;
import java.util.regex.*;

/** Activityの寿命に依存しない、公式APKの再開可能な取得。 */
final class PackageDownload {
    private static final long LIMIT=100L*1024*1024;
    static boolean validId(String id){return id.equals("termux")||id.equals("nightmare");}
    static String expected(String id){return id.equals("termux")?"72fdb596045116bf5ba1b5bdf5b26fddb9acc0bd074ad9f2da9eb0ae85e83a4e":"51d8366e1c644285d0f9d675170f11504b0623767d47b98a68216c068cbe2f1f";}
    static String sha(File file)throws Exception{
        MessageDigest d=MessageDigest.getInstance("SHA-256");try(InputStream in=new FileInputStream(file)){byte[] b=new byte[65536];int n;while((n=in.read(b))>0)d.update(b,0,n);}
        StringBuilder s=new StringBuilder();for(byte b:d.digest())s.append(String.format(Locale.ROOT,"%02x",b&255));return s.toString();
    }
    static class Paused extends IOException{Paused(String text){super(text);}}
    static void check(Context c,String id)throws Paused{String why=DownloadState.pauseReason(c,id);if(!why.isEmpty())throw new Paused(why);}
    static void run(Context c,String id){
        if(!validId(id))return;
        File target=new File(c.getCacheDir(),id+".apk"),part=new File(c.getCacheDir(),id+".apk.part");
        String url=id.equals("termux")?"https://github.com/termux/termux-app/releases/download/v0.118.3/termux-app_v0.118.3%2Bgithub-debug_arm64-v8a.apk":"https://github.com/AbrahamPaulJ/nightmare-mobile/releases/download/v1.6.211/nightmare-1.6.211.apk";
        try{
            if(part.isFile()&&sha(part).equals(expected(id))&&!part.renameTo(target))throw new IOException("APKを保存できませんでした。");
            if(!target.isFile()||!sha(target).equals(expected(id))){
                check(c,id);long start=part.length();
                if(start>LIMIT)throw new IOException("途中ファイルが不正です。「取り直す」を選んでください。");
                HttpURLConnection conn=(HttpURLConnection)new URL(url).openConnection();conn.setConnectTimeout(20000);conn.setReadTimeout(20000);
                conn.setRequestProperty("Range","bytes="+start+"-");conn.setRequestProperty("Accept-Encoding","identity");
                try{
                    int code=conn.getResponseCode();long total;
                    if(code==206){
                        Matcher m=Pattern.compile("bytes (\\d+)-(\\d+)/(\\d+)").matcher(String.valueOf(conn.getHeaderField("Content-Range")));
                        if(!m.matches()||Long.parseLong(m.group(1))!=start)throw new IOException("再開位置を確認できませんでした。");
                        total=Long.parseLong(m.group(3));
                        if(total<=start||Long.parseLong(m.group(2))!=total-1)throw new IOException("取得サイズが一致しません。");
                    }else if(code==200){start=0;total=conn.getContentLengthLong();}
                    else throw new IOException("取得元への接続に失敗しました（HTTP "+code+"）。");
                    if(total>LIMIT)throw new IOException("APKのサイズが上限を超えています。");
                    long size=start,initial=start,began=System.nanoTime(),last=0;
                    try(InputStream in=conn.getInputStream();OutputStream out=new FileOutputStream(part,start>0)){
                        byte[] buffer=new byte[65536];int n;
                        while((n=in.read(buffer))!=-1){
                            check(c,id);size+=n;if(size>LIMIT||(total>0&&size>total))throw new IOException("取得サイズが上限を超えました。");out.write(buffer,0,n);
                            long now=System.nanoTime();if(now-last>750000000L){
                                long speed=(long)((size-initial)/Math.max(.1,(now-began)/1e9));
                                DownloadState.put(c,id,"state","running","phase","download","message","公式APKを取得しています","bytes",size,"total",total,"speed",speed,"eta",speed>0&&total>0?(total-size)/speed:-1);last=now;
                            }
                        }
                    }
                    if(total>0&&size!=total)throw new IOException("通信が中断されました。続きから再開できます。");
                }finally{conn.disconnect();}
                DownloadState.put(c,id,"state","running","phase","verify","message","配布元のSHA-256と照合しています");
                if(!sha(part).equals(expected(id)))throw new IOException("ファイル確認に失敗しました。「取り直す」を選んでください。");
                if(!part.renameTo(target))throw new IOException("APKを保存できませんでした。");
            }
            DownloadState.put(c,id,"state","ready","phase","ready","message","確認済みです。インストールできます。","bytes",target.length(),"total",target.length());
        }catch(Paused e){DownloadState.put(c,id,"state","paused","message",e.getMessage(),"speed",0,"eta",-1);}
        catch(Exception e){DownloadState.put(c,id,"state","error","message",e.getMessage()==null?"通信を確認して再開してください。":e.getMessage(),"speed",0,"eta",-1);}
    }
}
