package io.textile.textileexample;

import android.os.Bundle;
import android.support.v7.app.AppCompatActivity;
import android.view.View;

import io.textile.threads.Client;
import io.textile.threads.Config;
import io.textile.threads.DefaultConfig;

public class MainActivity extends AppCompatActivity {

    Client client;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        System.out.println("Info: " + "Startup");
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        initIPFS();
    }

    public void onButtonClick(View v) {
        try {
            client.NewDBSync("bafk7ayo2xuuafgx6ubbcn2lro3s7oixgujdda6shv4");
            System.out.println("Added DB");
        } catch (Exception e) {
            System.out.println("Error Info: " + e.getMessage());
        }
    }

    private void initIPFS() {
        try {
            /**
             * To use hosted threads from Textile, use the following instead of DefaultConfig
             */
            /*
                Config config = new TextileConfig(
                    PROJECT_TOKEN,
                    DEVICE_UUID
                );
                // Optionally, restore a session for the same user by supplying the session ID
                config.setSession(SESSION_ID);
                ...
                client = new Client(config);
                ...
                // If it's your first time starting the client, you can get and locally store the session id
                String SESSION_ID = client.getSession();
                // The above only works when using the TextileConfig
            */

            Config config = new DefaultConfig();
            client = new Client(config);
            client.init().get();
            System.out.println("Thread info: success");
        } catch (Exception e) {
            System.out.println("Thread info: " + e.getMessage());
        }
    }
}
