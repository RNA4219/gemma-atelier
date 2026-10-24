package local.gemma.launcher;
import java.nio.charset.StandardCharsets;
import java.io.*;
import java.util.*;
public class TermuxLinkTest {
    public static void main(String[] args)throws Exception{
        for(String value:new String[]{"","# existing\nfont-size=15\n","allow-external-apps = false\n","allow-external-apps:true\n","allow-external-apps=false\nallow-external-apps=true\n","custom=continued\\"}){
            byte[] before=value.getBytes(StandardCharsets.UTF_8),after=TermuxLink.enable(before);
            if(!new String(after,StandardCharsets.UTF_8).startsWith(value))throw new AssertionError("既存設定が失われた");
            Properties p=new Properties();p.load(new InputStreamReader(new ByteArrayInputStream(after),StandardCharsets.UTF_8));
            if(!"true".equals(p.getProperty("allow-external-apps")))throw new AssertionError("接続が無効");
            if(!Arrays.equals(after,TermuxLink.enable(after)))throw new AssertionError("再実行で変更された");
        }
        if(CommandResultReceiver.classify(-1,0)!=0||CommandResultReceiver.classify(-1,1)!=2||CommandResultReceiver.classify(0,0)!=1||CommandResultReceiver.classify(2,0)!=1)throw new AssertionError("Termux成功・終了コード・拒否・取消の区別");
        System.out.println("PASS: Termux設定6条件、既存内容保持・有効化・冪等性、結果コード4条件");
    }
}
