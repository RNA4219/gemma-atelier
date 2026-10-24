package local.gemma.launcher;

import android.content.*;
import android.net.*;
import org.json.*;
import java.util.Iterator;

/** 表示とサービスで共有する、秘密値を含まないダウンロード記録。 */
final class DownloadState {
    static android.content.SharedPreferences prefs(Context c){return c.getSharedPreferences("setup",0);}
    static synchronized JSONObject job(Context c,String id){
        try{return new JSONObject(prefs(c).getString("job_"+id,"{}"));}catch(JSONException e){return new JSONObject();}
    }
    static synchronized void update(Context c,String id,JSONObject patch){
        JSONObject value=job(c,id);
        try{
            if(patch.has("phase")&&!patch.optString("phase").equals(value.optString("phase")))
                value.put("bytes",0).put("total",0).put("speed",0).put("eta",-1);
            Iterator<String> keys=patch.keys();while(keys.hasNext()){String k=keys.next();value.put(k,patch.get(k));}
            value.put("updated_at",System.currentTimeMillis()/1000.0);
            prefs(c).edit().putString("job_"+id,value.toString()).apply();
        }catch(JSONException ignored){}
    }
    static void put(Context c,String id,Object... fields){
        JSONObject o=new JSONObject();try{for(int i=0;i<fields.length;i+=2)o.put((String)fields[i],fields[i+1]);}catch(JSONException ignored){}update(c,id,o);
    }
    static boolean wifi(Context c){
        ConnectivityManager m=c.getSystemService(ConnectivityManager.class);
        NetworkCapabilities n=m.getNetworkCapabilities(m.getActiveNetwork());
        return n!=null&&(n.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)||n.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET));
    }
    static String pauseReason(Context c,String id){
        if(prefs(c).getString("command_"+id,"run").equals("pause"))return "一時停止しました。途中のデータは保存されています。";
        if(prefs(c).getBoolean("wifi_only",true)&&!wifi(c))return "Wi-Fi接続待ちです。接続後に「再開」を押してください。";
        return "";
    }
    static boolean busy(Context c){
        for(String id:new String[]{"bolt","termux","nightmare"})if(job(c,id).optString("state").equals("running"))return true;
        return false;
    }
}
