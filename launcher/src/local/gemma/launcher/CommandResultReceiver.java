package local.gemma.launcher;

import android.content.*;
import android.os.Bundle;

/** Termuxの拒否・終了を受け、無期限の「準備中」にしない。出力には秘密値があり得るので保存しない。 */
public class CommandResultReceiver extends BroadcastReceiver {
    // Termuxのerr成功値はPOSIXの0ではなくActivity.RESULT_OK（-1）。exitCodeは別の値。
    static int classify(int error,int exit){return error!=android.app.Activity.RESULT_OK?1:exit!=0?2:0;}
    @Override public void onReceive(Context context,Intent intent){
        String request=intent.getStringExtra("request");
        if(request==null||!request.equals(DownloadState.prefs(context).getString("bootstrap_request","")))return;
        Bundle result=intent.getBundleExtra("result");if(result==null)return;
        int outcome=classify(result.getInt("err",android.app.Activity.RESULT_CANCELED),result.getInt("exitCode",0));
        if(outcome==0)return;
        if(outcome==1){
            DownloadState.prefs(context).edit().putBoolean("command_ready",false).putBoolean("termux_linked",false).apply();
            DownloadState.put(context,"bolt","state","error","error_code","termux_link","phase","connection","message","Termuxとの接続が許可されていません。「接続を設定し直す」から許可を確認してください。","speed",0,"eta",-1);
        }else if(!DownloadState.job(context,"bolt").optString("state").equals("error")){
            DownloadState.put(context,"bolt","state","error","error_code","bootstrap","message","準備が途中で終了しました。通信と空き容量を確認して、続きから再開してください。","speed",0,"eta",-1);
        }
    }
}
