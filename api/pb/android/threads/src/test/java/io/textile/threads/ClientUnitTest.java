package io.textile.threads;

import org.json.JSONObject;
import org.junit.Test;
import org.junit.FixMethodOrder;
import org.junit.runner.RunWith;
import org.junit.runners.JUnit4;
import org.junit.runners.MethodSorters;


import java.io.BufferedReader;
import java.io.FileInputStream;
import java.io.InputStreamReader;
import java.util.ArrayList;

import com.google.common.io.BaseEncoding;
import com.google.gson.Gson;
import com.google.protobuf.ByteString;

import io.textile.threads_grpc.*;

import static org.junit.Assert.*;

@RunWith(JUnit4.class)
@FixMethodOrder(MethodSorters.NAME_ASCENDING)
public class ClientUnitTest {

    static Client client;
    static String dbId = "AVXwYdq9KAKa/qBCJulxduX3IuaiRjB6R68=";
    static String instanceId = "";

    void connect() throws Exception {
        // Initialize & start
        client = new Client();
        // Await initialization
        client.init().get();
        System.out.println("Thread Info: " + "READY!");
    }

    @Test
    public void t01_StartTest() throws Exception {
        connect();
        assertEquals(true, client.connected());
    }

    @Test
    public void t02_NewDB() throws Exception {
        client.NewDBSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), new ArrayList<>());
    }

    @Test
<<<<<<< HEAD
    public void t04_GetInviteInfo() throws Exception {
        Credentials.Builder creds = Credentials.newBuilder();
        creds.setThreadID(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)));
        GetInviteInfoReply reply = client.GetInviteInfoSync(creds.build());
        assertNotEquals(0, reply.getAddressesCount());
=======
    public void t04_GetDBInfo() throws Exception {
        GetDBInfoReply reply = client.GetDBInfoSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)));
        assertNotEquals(0, reply.getAddrsCount());
>>>>>>> origin/master
    }

    @Test
    public void t05_NewCollection() throws Exception {
        String jsonStr = getStoredSchema();
        ByteString schema = ByteString.copyFrom(jsonStr.getBytes());
        JSONObject json = new JSONObject(jsonStr);
        assertEquals(json.get("title").toString(), "Person");
        client.NewCollectionSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), "Person", schema);
    }

    @Test
    public void t06_Create() throws Exception {
        ByteString person = createPerson("", 22);
        ByteString[] data = { person };
        CreateReply reply = client.CreateSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), "Person", data);
        assertEquals(1, reply.getInstanceIDsCount());
        String id = reply.getInstanceIDs(0);
        instanceId = id;
        assertEquals(id.length(), 36);
    }

    @Test
    public void t06_Save() throws Exception {
        ByteString person = createPerson(instanceId, 22);
        ByteString[] data = { person };
        client.SaveSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), "Person", data);
        // now check that it's been updated
        FindByIDReply reply = client.FindByIDSync(ByteString.copyFrom(BaseEncoding.base64().decode(dbId)), "Person", instanceId);
        ByteString jsonBytes = reply.getInstance();
        Person instance = new Gson().fromJson(jsonBytes.toStringUtf8(), Person.class);
        assertEquals(instanceId, instance.ID);
    }

    private ByteString createPerson(String ID, int age) throws Exception {
        Gson gson = new Gson();
        Person person = new Person(ID, age);
        String json = gson.toJson(person);
        return ByteString.copyFrom(json.getBytes());
    }

    private String getStoredSchema() throws Exception {
        BufferedReader br = new BufferedReader(new InputStreamReader(new FileInputStream("./src/test/resources/person.json")));
        StringBuilder sb = new StringBuilder();
        String line = br.readLine();
        while (line != null) {
            sb.append(line);
            line = br.readLine();
        }
        return sb.toString();
    }
}

class Person {
    public String firstName = "adam";
    public String lastName = "doe";
    public String ID;
    public int age;
    Person(String ID, int age) {
        this.age = age;
        this.ID = ID;
    }
}

