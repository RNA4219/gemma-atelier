package local.gemma.launcher;

import android.content.*;
import android.database.Cursor;
import android.net.Uri;
import android.provider.DocumentsContract;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Androidのフォルダ許可で選ばれたTermux設定だけを変更する。 */
final class TermuxLink {
    static final String AUTHORITY="com.termux.documents";
    static final String HOME="/data/data/com.termux/files/home";
    static Intent picker(){
        return new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
            .putExtra(DocumentsContract.EXTRA_INITIAL_URI,DocumentsContract.buildDocumentUri(AUTHORITY,HOME))
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION|Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
    }
    private static Uri child(ContentResolver resolver,Uri tree,String parent,String name)throws IOException{
        Uri list=DocumentsContract.buildChildDocumentsUriUsingTree(tree,parent);
        try(Cursor cursor=resolver.query(list,new String[]{DocumentsContract.Document.COLUMN_DOCUMENT_ID,DocumentsContract.Document.COLUMN_DISPLAY_NAME},null,null,null)){
            if(cursor==null)throw new IOException("Termuxのフォルダを読み取れませんでした。");
            while(cursor.moveToNext())if(name.equals(cursor.getString(1)))
                return DocumentsContract.buildDocumentUriUsingTree(tree,cursor.getString(0));
        }return null;
    }
    private static byte[] read(ContentResolver resolver,Uri file)throws IOException{
        try(InputStream in=resolver.openInputStream(file);ByteArrayOutputStream out=new ByteArrayOutputStream()){
            if(in==null)throw new IOException("Termuxの設定を開けませんでした。");
            byte[] b=new byte[4096];int n;while((n=in.read(b))!=-1){if(out.size()+n>262144)throw new IOException("設定ファイルが大きすぎるため変更を中止しました。");out.write(b,0,n);}return out.toByteArray();
        }
    }
    private static void write(ContentResolver resolver,Uri file,byte[] value)throws IOException{
        try(OutputStream out=resolver.openOutputStream(file,"rwt")){
            if(out==null)throw new IOException("Termuxの設定へ書き込めませんでした。");out.write(value);
        }
    }
    static byte[] enable(byte[] previous)throws IOException{
        Properties properties=new Properties();properties.load(new InputStreamReader(new ByteArrayInputStream(previous),StandardCharsets.UTF_8));
        if("true".equals(properties.getProperty("allow-external-apps","false").trim()))return previous;
        ByteArrayOutputStream out=new ByteArrayOutputStream();out.write(previous);
        out.write("\n\n# LLM Atelier: Androidで許可したアプリからの起動連携\nallow-external-apps = true\n".getBytes(StandardCharsets.UTF_8));
        return out.toByteArray();
    }
    static void configure(Context context,Uri tree)throws IOException{
        if(tree==null||!AUTHORITY.equals(tree.getAuthority())||!DocumentsContract.isTreeUri(tree))throw new IOException("左上のメニューから「Termux」を選んでください。");
        String selected=DocumentsContract.getTreeDocumentId(tree);
        if(!selected.equals(HOME)&&!selected.equals(HOME+"/.termux"))throw new IOException("Termuxの一番上のフォルダを選んでください。Downloadフォルダでは接続できません。");
        ContentResolver resolver=context.getContentResolver();Uri directory=DocumentsContract.buildDocumentUriUsingTree(tree,selected);
        if(selected.equals(HOME)){
            Uri found=child(resolver,tree,HOME,".termux");
            directory=found!=null?found:DocumentsContract.createDocument(resolver,directory,DocumentsContract.Document.MIME_TYPE_DIR,".termux");
        }
        if(directory==null)throw new IOException("Termuxを一度開き、初期化が終わってから接続してください。");
        String parent=DocumentsContract.getDocumentId(directory);
        Uri file=child(resolver,tree,parent,"termux.properties");
        byte[] previous=file==null?new byte[0]:read(resolver,file),next=enable(previous);
        if(java.util.Arrays.equals(previous,next))return;
        // 設定は書き換える前に同じフォルダへ退避。既存のバックアップは上書きしない。
        Uri backup=DocumentsContract.createDocument(resolver,directory,"text/plain","termux.properties.atelier-backup");
        if(backup==null)throw new IOException("既存設定を保存できないため変更を中止しました。");
        write(resolver,backup,previous);
        if(!java.util.Arrays.equals(read(resolver,backup),previous))throw new IOException("既存設定の保存を確認できませんでした。");
        if(file==null)file=DocumentsContract.createDocument(resolver,directory,"text/plain","termux.properties");
        if(file==null)throw new IOException("Termuxの設定を作成できませんでした。");
        try{
            write(resolver,file,next);
            if(!java.util.Arrays.equals(read(resolver,file),next))throw new IOException("設定の書き込みを確認できませんでした。");
        }catch(IOException error){try{write(resolver,file,previous);}catch(IOException restore){error.addSuppressed(restore);}throw error;}
    }
}
