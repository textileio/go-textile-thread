package io.textile.threads;

import android.support.test.runner.AndroidJUnit4;

import com.google.common.io.BaseEncoding;
import com.google.protobuf.ByteString;

import org.junit.FixMethodOrder;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.junit.runners.MethodSorters;

import java.util.ArrayList;
import java.util.logging.Logger;

import static org.junit.Assert.assertEquals;

/**
 * Textile tests.
 */
@RunWith(AndroidJUnit4.class)
@FixMethodOrder(MethodSorters.NAME_ASCENDING)
public class ClientTest {

    private final static Logger logger =
            Logger.getLogger("TEST");

    static Client client;

    void connect() throws Exception {
        // Start
        client = new Client();
        client.init().get();
    }

    @Test
    public void startTest() throws Exception {
        connect();
        assertEquals(true, client.connected());
    }

    @Test
    public void NewDB() throws Exception {
        if (client == null) {
            connect();
        }

        String dbId = "AVXwYdq9KAKa/qBCJulxduX3IuaiRjB6R68=";
        client.NewDBSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), new ArrayList<>());
    }
}
