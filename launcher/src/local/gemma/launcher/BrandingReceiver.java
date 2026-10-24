package local.gemma.launcher;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

public final class BrandingReceiver extends BroadcastReceiver {
    @Override public void onReceive(Context context,Intent intent){
        if(Intent.ACTION_MY_PACKAGE_REPLACED.equals(intent.getAction()))ShortcutBranding.refresh(context);
    }
}
