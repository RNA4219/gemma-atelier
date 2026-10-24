package local.gemma.launcher;

import android.content.Context;
import android.content.Intent;
import android.content.pm.ShortcutInfo;
import android.content.pm.ShortcutManager;
import android.graphics.drawable.Icon;
import java.util.Collections;

/** 既存の固定ショートカットを同じIDのまま更新する。 */
final class ShortcutBranding {
    static final String NAME="LLM Atelier";
    static ShortcutInfo shortcut(Context context){
        return new ShortcutInfo.Builder(context,"bolt-home")
            .setShortLabel(NAME).setLongLabel(NAME)
            .setIcon(Icon.createWithResource(context,context.getApplicationInfo().icon))
            .setIntent(new Intent(context,MainActivity.class).setAction(Intent.ACTION_MAIN)).build();
    }
    static void refresh(Context context){
        try{
            ShortcutManager manager=context.getSystemService(ShortcutManager.class);
            for(ShortcutInfo item:manager.getPinnedShortcuts()){
                if(item.getId().equals("bolt-home")&&(!NAME.contentEquals(item.getShortLabel())||item.getLongLabel()==null||!NAME.contentEquals(item.getLongLabel()))){
                    manager.updateShortcuts(Collections.singletonList(shortcut(context)));break;
                }
            }
        }catch(RuntimeException ignored){/* 起動時にも再試行する。既存アイコンは削除しない。 */}
    }
}
